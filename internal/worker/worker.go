package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/worker/backends"
)

type Worker struct {
	client   *client.Client
	config   *config.Config
	backends *backends.Backends
	nodeID   string
	stream   jetstream.Stream
	consumer jetstream.Consumer
	context  jetstream.ConsumeContext
	stopHB   context.CancelFunc
}

func New(cfg *config.Config) *Worker {
	return &Worker{
		config:   cfg,
		backends: backends.New(cfg.Worker.AllowedPaths),
		nodeID:   cfg.NATS.Name,
	}
}

func (w *Worker) Start() error {
	log.Info("Starting worker", "node", w.nodeID)

	// Create client
	var err error
	if w.client, err = client.New(w.config, nil); err != nil {
		return fmt.Errorf("creating client: %w", err)
	}
	defer w.client.Close()

	// Register this node
	if err := w.register(); err != nil {
		return fmt.Errorf("registering node: %w", err)
	}

	// Start heartbeat
	hbCtx, hbCancel := context.WithCancel(context.Background())
	w.stopHB = hbCancel
	go w.heartbeat(hbCtx)

	// Get commands stream
	w.stream, err = w.client.GetStream("commands")
	if err != nil {
		return fmt.Errorf("getting commands stream: %w", err)
	}

	// Build filter subjects based on groups and node ID
	filters := w.buildFilters()
	log.Debug("Consumer filters", "filters", filters)

	// Parse configurable inactive threshold (default: 10m)
	inactiveThreshold := 10 * time.Minute
	if w.config.Worker.InactiveThreshold != "" {
		if d, err := time.ParseDuration(w.config.Worker.InactiveThreshold); err == nil {
			inactiveThreshold = d
		} else {
			log.Warn("Invalid inactive_threshold, using default", "value", w.config.Worker.InactiveThreshold)
		}
	}

	// Create durable consumer
	consumerName := fmt.Sprintf("worker-%s", w.nodeID)
	w.consumer, err = w.client.EnsureConsumer(w.stream, jetstream.ConsumerConfig{
		Durable:           consumerName,
		FilterSubjects:    filters,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		InactiveThreshold: inactiveThreshold,
		MaxAckPending:     1, // FIFO: one command at a time per worker
	})
	if err != nil {
		return fmt.Errorf("creating consumer: %w", err)
	}

	// Start consuming
	consOpts := []jetstream.PullConsumeOpt{
		jetstream.ConsumeErrHandler(func(ctx jetstream.ConsumeContext, err error) {
			log.Error("Consumer error", "error", err)
		}),
	}
	w.context, err = w.consumer.Consume(w.handleCommand, consOpts...)
	if err != nil {
		return fmt.Errorf("starting consumer: %w", err)
	}

	log.Info("Worker started", "node", w.nodeID, "groups", w.config.Worker.Groups, "backends", w.backends.List())
	common.WaitForSignal()

	w.Stop()
	return nil
}

func (w *Worker) Stop() {
	log.Info("Stopping worker", "node", w.nodeID)

	if w.stopHB != nil {
		w.stopHB()
	}

	if w.context != nil {
		w.context.Stop()
	}

	// Mark node offline
	w.deregister()

	if w.client != nil {
		w.client.Close()
	}

	log.Info("Worker stopped")
}

func (w *Worker) register() error {
	hostname, _ := os.Hostname()
	info := models.NodeInfo{
		ID:       w.nodeID,
		Hostname: hostname,
		Groups:   w.config.Worker.Groups,
		Backends: w.backends.List(),
		Status:   "online",
		LastSeen: time.Now().UTC(),
	}

	return w.client.PutNode(info)
}

func (w *Worker) deregister() {
	info := models.NodeInfo{
		ID:       w.nodeID,
		Status:   "offline",
		LastSeen: time.Now().UTC(),
	}
	if err := w.client.PutNode(info); err != nil {
		log.Error("Failed to deregister", "error", err)
	}
}

func (w *Worker) heartbeat(ctx context.Context) {
	interval := 30 * time.Second
	if w.config.Worker.HeartbeatInterval != "" {
		if d, err := time.ParseDuration(w.config.Worker.HeartbeatInterval); err == nil {
			interval = d
		} else {
			log.Warn("Invalid heartbeat_interval, using default", "value", w.config.Worker.HeartbeatInterval)
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.register(); err != nil {
				log.Error("Heartbeat failed", "error", err)
			}
		}
	}
}

// buildFilters creates the set of NATS subject filters for this worker's consumer.
func (w *Worker) buildFilters() []string {
	filters := []string{
		"cmd.all.>",
		fmt.Sprintf("cmd.node.%s.>", w.nodeID),
	}
	for _, group := range w.config.Worker.Groups {
		filters = append(filters, fmt.Sprintf("cmd.group.%s.>", group))
	}
	return filters
}

func (w *Worker) handleCommand(msg jetstream.Msg) {
	backend := msg.Headers().Get("backend")
	action := msg.Headers().Get("action")
	jobID := msg.Headers().Get("job-id")
	taskIndexStr := msg.Headers().Get("task-index")

	taskIndex, _ := strconv.Atoi(taskIndexStr)

	log.Info("Received command", "job", jobID, "step", taskIndex, "backend", backend, "action", action)

	// Decode params
	var params map[string]string
	if err := json.Unmarshal(msg.Data(), &params); err != nil {
		log.Error("Failed to decode params", "error", err)
		w.publishResult(jobID, taskIndex, models.ResultFailed, "", fmt.Sprintf("decode params: %s", err))
		msg.Ack()
		return
	}

	// Get backend
	b, ok := w.backends.Get(backend)
	if !ok {
		log.Error("Unknown backend", "backend", backend)
		w.publishResult(jobID, taskIndex, models.ResultFailed, "", fmt.Sprintf("unknown backend: %s", backend))
		msg.Ack()
		return
	}

	// Execute
	start := time.Now()
	result, err := b.Run(context.Background(), action, params)
	duration := time.Since(start)

	output := ""
	if result != nil {
		output = result.Output
	}

	if err != nil {
		log.Error("Task failed", "job", jobID, "step", taskIndex, "error", err, "duration", duration)
		w.publishResult(jobID, taskIndex, models.ResultFailed, output, err.Error())
	} else {
		log.Info("Task succeeded", "job", jobID, "step", taskIndex, "duration", duration)
		w.publishResult(jobID, taskIndex, models.ResultSuccess, output, "")
	}

	msg.Ack()
}

func (w *Worker) publishResult(jobID string, taskIndex int, status models.ResultStatus, output, errMsg string) {
	result := models.TaskResult{
		JobID:     jobID,
		TaskIndex: taskIndex,
		NodeID:    w.nodeID,
		Status:    status,
		Output:    output,
		Error:     errMsg,
		Timestamp: time.Now().UTC(),
	}

	if err := w.client.PublishResult(result); err != nil {
		log.Error("Failed to publish result", "error", err)
	}
}
