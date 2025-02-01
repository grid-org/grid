package controller

import (
	"fmt"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Controller struct {
	config *config.Config
	client *client.Client
}

func New(cfg *config.Config, c *client.Client) *Controller {
	return &Controller{
		config: cfg,
		client: c,
	}
}

func (c *Controller) Start() error {
	// Ensure streams
	log.Info("Ensuring requests stream")
	requests, err := c.client.EnsureStream(jetstream.StreamConfig{
		Name:      "requests",
		Subjects:  []string{"request.>"},
		Retention: jetstream.WorkQueuePolicy, // Only deliver to one controller
		Discard:   jetstream.DiscardOld,
		Replicas:  1,
	})
	if err != nil {
		return fmt.Errorf("Error ensuring stream: %w", err)
	}

	log.Info("Ensuring job stream")
	if _, err := c.client.EnsureStream(jetstream.StreamConfig{
		Name:      "jobs",
		Subjects:  []string{"job.*.>"},
		Retention: jetstream.InterestPolicy, // Remove acknowledged messages
		Discard:   jetstream.DiscardNew,
		MaxMsgs:   1, // Only one job at a time
		Replicas:  1,
	}); err != nil {
		return fmt.Errorf("Error ensuring stream: %w", err)
	}

	// Create consumer
	log.Info("Creating consumer")
	cons, err := c.client.EnsureConsumer(requests, jetstream.ConsumerConfig{
		Durable:        "controller",
		FilterSubjects: []string{"request.>"},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxAckPending:  1,
	})
	if err != nil {
		return fmt.Errorf("Error creating consumer: %w", err)
	}

	// Create cluster bucket
	log.Info("Creating key-value bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "cluster",
	}); err != nil {
		return fmt.Errorf("Error creating key-value bucket: %w", err)
	}

	// Store cluster status
	log.Info("Storing cluster status")
	if err := c.client.PutKV("cluster", "status", []byte("active")); err != nil {
		return fmt.Errorf("Error storing cluster status: %w", err)
	}

	// Create jobs bucket
	log.Info("Ensuring jobs bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "jobs",
	}); err != nil {
		return fmt.Errorf("Error ensuring jobs bucket: %w", err)
	}

	// Start consuming
	log.Info("Consuming requests")
	if _, err := cons.Consume(c.handleRequest); err != nil {
		return fmt.Errorf("Error consuming: %w", err)
	}

	log.Info("Controller started")
	return nil
}

func (c *Controller) handleRequest(msg jetstream.Msg) {
	meta, err := msg.Metadata()
	if err != nil {
		log.Error("Error getting metadata", "error", err)
		msg.TermWithReason(fmt.Sprintf("error getting metadata: %v", err))
		return
	}

	job := client.Job{
		ID: 	 meta.Sequence.Stream,
		Backend: msg.Headers().Get("backend"),
		Action:  msg.Headers().Get("action"),
		Payload: string(msg.Data()),
		Timestamp: meta.Timestamp,
	}

	log.Info("Received request",
		"id", job.ID,
		"backend", job.Backend,
		"action", job.Action,
		"payload", job.Payload,
		"timestamp", job.Timestamp.Format(time.RFC3339),
	)

	// Push to job stream
	jobSubject := fmt.Sprintf("job.all.%s", job.Backend)
	jobMsg := &nats.Msg{
		Subject: jobSubject,
		Data:    msg.Data(),
		Header:  msg.Headers(),
	}
	if _, err := c.client.Publish(jobMsg); err != nil {
		// Will retry after a delay
		if err := msg.NakWithDelay(time.Second * 5); err != nil {
			log.Error("Error nacking request with delay", "error", err)
		} else {
			log.Info("Nacked request with delay", "id", job.ID, "backend", job.Backend, "action", job.Action)
		}
	} else {
		// Acknowledge
		if err := msg.Ack(); err != nil {
			log.Error("Error acknowledging request", "error", err)
		} else {
			log.Info("Acknowledged request", "id", job.ID, "backend", job.Backend, "action", job.Action)
		}
	}
}
