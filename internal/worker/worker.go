package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/nats-io/nats.go/jetstream"
)

func Run(cfg *config.Config, client *client.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Setup consumer for jobs
	cons, err := client.JS.CreateConsumer(ctx, "jobs", jetstream.ConsumerConfig{
		FilterSubjects: []string{
			"job.all.>",
		},
		DeliverPolicy: jetstream.DeliverLastPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		MaxAckPending: 1, // Only one job at a time
	})
	if err != nil {
		return fmt.Errorf("Error creating consumer: %w", err)
	}

	// Start consuming
	_, err = cons.Consume(func(msg jetstream.Msg) {
		handleJob(client, msg)
	})
	if err != nil {
		return fmt.Errorf("Error consuming: %w", err)
	}

	log.Info("Worker started")
	common.WaitForSignal()
	return nil
}

func handleJob(c *client.Client, msg jetstream.Msg) {
	var req client.Request
	if err := json.Unmarshal(msg.Data(), &req.Payload); err != nil {
		log.Error("Error unmarshalling job", "error", err)
		msg.TermWithReason("invalid payload")
		return
	}

	req.Action = msg.Headers().Get("action")
	log.Info("Received job", "subject", msg.Subject(), "payload", req.Payload, "action", req.Action)

	// msg.InProgress()

	// Do the job
	time.Sleep(5 * time.Second)

	// Notify that the job is done
	msg.Ack()
}
