package worker

import (
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/worker/backends"
)

type Worker struct {
	client     *client.Client
	config     *config.Config
	backends   *backends.Backends
	stream     jetstream.Stream
	consumer   jetstream.Consumer
	context    jetstream.ConsumeContext
	inProgress bool
}

func New(cfg *config.Config) *Worker {
	return &Worker{
		config:   cfg,
		backends: backends.New(),
	}
}

func (w *Worker) Start() error {
	log.Info("Starting worker")

	log.Debug("Worker config", "config", w.config)
	log.Debug("Worker backends", "backends", w.backends)

	// Create client
	log.Debug("Creating client")
	var err error
	if w.client, err = client.New(w.config, nil); err != nil {
		return fmt.Errorf("Error creating client: %w", err)
	}
	defer w.client.Close()

	// Get stream
	w.stream, err = w.client.GetStream("jobs")
	if err != nil {
		return fmt.Errorf("Error getting stream: %w", err)
	}

	// Setup consumer for jobs
	log.Debug("Setting up consumer")
	w.consumer, err = w.client.EnsureConsumer(w.stream, jetstream.ConsumerConfig{
		Durable: w.config.NATS.Name,
		FilterSubjects: []string{
			"job.all.>",
			// "job." + w.config.NATS.Name + ".>",
		},
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		MaxAckPending:     1, // Only one job at a time
		InactiveThreshold: 10 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("Error creating consumer: %w", err)
	}

	// Start consuming
	log.Debug("Starting consumer")
	consOpts := []jetstream.PullConsumeOpt{
		jetstream.ConsumeErrHandler(func(ctx jetstream.ConsumeContext, err error) {
			log.Error("Error consuming message", "error", err)
		}),
	}
	w.context, err = w.consumer.Consume(w.handleJob, consOpts...)
	if err != nil {
		return fmt.Errorf("Error consuming: %w", err)
	}

	log.Info("Worker started")
	common.WaitForSignal()
	return nil
}

func (w *Worker) Stop() {
	log.Info("Stopping worker")

	// Stop consuming
	w.context.Stop()

	// Delete consumer
	if err := w.client.DeleteConsumer(w.stream, w.config.NATS.Name); err != nil {
		log.Error("Error deleting consumer", "error", err)
	}

	// Close client
	w.client.Close()

	log.Info("Worker stopped")
}

func (w *Worker) handleJob(msg jetstream.Msg) {
	if w.inProgress {
		msg.InProgress()
		return
	}

	job := client.Job{
		Backend: msg.Headers().Get("backend"),
		Action:  msg.Headers().Get("action"),
		Payload: string(msg.Data()),
	}

	log.Info("Received job", "subject", msg.Subject(), "backend", job.Backend, "payload", job.Payload, "action", job.Action)

	// Process the job
	w.inProgress = true
	if backend, ok := w.backends.Get(job.Backend); ok {
		if err := backend.Run(job); err != nil {
			log.Error("Error processing job", "error", err)
			msg.TermWithReason("processing error")
			w.inProgress = false
			return
		}
	} else {
		log.Error("Unknown backend", "backend", job.Backend)
		msg.TermWithReason("unknown backend")
		w.inProgress = false
		return
	}

	// Notify that the job is done
	log.Info("Job done", "subject", msg.Subject(), "backend", job.Backend, "payload", job.Payload, "action", job.Action)
	msg.Ack()
	w.inProgress = false
}
