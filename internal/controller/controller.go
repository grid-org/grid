package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func Run(cfg *config.Config, c *client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure streams
	log.Info("Ensuring requests stream")
	requests, err := c.JS.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
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
	_, err = c.JS.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      "jobs",
		Subjects:  []string{"job.*.>"},
		Retention: jetstream.InterestPolicy, // Remove acknowledged messages
		Discard:   jetstream.DiscardNew,
		MaxMsgs:   1, // Only one job at a time
		Replicas:  1,
	})
	if err != nil {
		return fmt.Errorf("Error ensuring stream: %w", err)
	}

	// Create consumer
	log.Info("Creating consumer")
	cons, err := requests.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Durable:        "controller",
		FilterSubjects: []string{"request.>"},
		DeliverPolicy:  jetstream.DeliverAllPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxAckPending:  1,
	})
	if err != nil {
		return fmt.Errorf("Error creating consumer: %w", err)
	}

	// Create key-value bucket
	log.Info("Creating key-value bucket")
	bucket, err := c.JS.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "cluster",
	})
	if err != nil {
		return fmt.Errorf("Error creating key-value bucket: %w", err)
	}

	// Store cluster status
	log.Info("Storing cluster status")
	_, err = bucket.Put(ctx, "status", []byte("Grid"))
	if err != nil {
		return fmt.Errorf("Error storing cluster status: %w", err)
	}

	// Start consuming
	log.Info("Consuming requests")
	_, err = cons.Consume(func(msg jetstream.Msg) {
		handleRequest(c, msg)
	})
	if err != nil {
		return fmt.Errorf("Error consuming: %w", err)
	}

	log.Info("Controller started")
	common.WaitForSignal()
	return nil
}

func handleRequest(c *client.Client, msg jetstream.Msg) {
	var req client.Request
	if err := json.Unmarshal(msg.Data(), &req.Payload); err != nil {
		log.Error("Error unmarshaling request", "error", err)
		msg.TermWithReason(fmt.Sprintf("error unmarshaling request: %v", err))
		return
	}

	meta, err := msg.Metadata()
	if err != nil {
		log.Error("Error getting metadata", "error", err)
		msg.TermWithReason(fmt.Sprintf("error getting metadata: %v", err))
		return
	}

	req.ID = meta.Sequence.Stream
	req.Timestamp = meta.Timestamp
	req.Action = msg.Headers().Get("action")

	log.Info("Received request",
		"id", req.ID,
		"action", req.Action,
		"payload", req.Payload,
		"timestamp", req.Timestamp.Format(time.RFC3339),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Push to job stream
	jobSubject := fmt.Sprintf("job.all.%s", req.Action)
	jobMsg := &nats.Msg{
		Subject: jobSubject,
		Data:    msg.Data(),
		Header:  msg.Headers(),
	}
	if _, err := c.JS.PublishMsg(ctx, jobMsg); err != nil {
        // Will retry after a delay
        if err := msg.NakWithDelay(time.Second * 5); err != nil {
            log.Error("Error nacking request with delay", "error", err)
        } else {
            log.Info("Nacked request with delay", "id", req.ID, "action", req.Action)
        }
    } else {
        // Acknowledge
        if err := msg.Ack(); err != nil {
            log.Error("Error acknowledging request", "error", err)
        } else {
            log.Info("Acknowledged request", "id", req.ID, "action", req.Action)
        }
    }
}
