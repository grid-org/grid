package worker

import (
	"encoding/json"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/worker/backends"
)

type Worker struct {
	client *client.Client
	config *config.Config
	backends *backends.Backends
}

func New(cfg *config.Config, client *client.Client) *Worker {
	return &Worker{
		client: client,
		config: cfg,
		backends: backends.New(),
	}
}

func (w *Worker) Start() error {
	log.Info("Starting worker")

	// Get stream
	stream, err := w.client.GetStream("jobs")
	if err != nil {
		return fmt.Errorf("Error getting stream: %w", err)
	}

	// Setup consumer for jobs
	cons, err := w.client.EnsureConsumer(stream, jetstream.ConsumerConfig{
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
	_, err = cons.Consume(w.handleJob)
	if err != nil {
		return fmt.Errorf("Error consuming: %w", err)
	}

	log.Info("Worker started")
	common.WaitForSignal()
	return nil
}

func (w *Worker) handleJob(msg jetstream.Msg) {
	var req client.Request
	if err := json.Unmarshal(msg.Data(), &req.Payload); err != nil {
		log.Error("Error unmarshalling job", "error", err)
		msg.TermWithReason("invalid payload")
		return
	}

	req.Action = msg.Headers().Get("action")
	log.Info("Received job", "subject", msg.Subject(), "payload", req.Payload, "action", req.Action)

	// Process the job
	if backend, ok := w.backends.Get(req.Action); ok {
		if err := backend.Run(req); err != nil {
			log.Error("Error processing job", "error", err)
			msg.TermWithReason("processing error")
			return
		}
	} else {
		log.Error("Unknown action", "action", req.Action)
		msg.TermWithReason("unknown action")
		return
	}

	// Notify that the job is done
	msg.Ack()
}
