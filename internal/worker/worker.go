package worker

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
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
	return nil
}

func (w *Worker) handleJob(msg jetstream.Msg) {
	job := client.Job{
		Backend: msg.Headers().Get("backend"),
		Action: msg.Headers().Get("action"),
		Payload: string(msg.Data()),
	}

	log.Info("Received job", "subject", msg.Subject(), "backend", job.Backend, "payload", job.Payload, "action", job.Action)

	// Process the job
	if backend, ok := w.backends.Get(job.Backend); ok {
		if err := backend.Run(job); err != nil {
			log.Error("Error processing job", "error", err)
			msg.TermWithReason("processing error")
			return
		}
	} else {
		log.Error("Unknown backend", "backend", job.Backend)
		msg.TermWithReason("unknown backend")
		return
	}

	// Notify that the job is done
	msg.Ack()
}
