package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const DefaultTaskTimeout = 5 * time.Minute

// Scheduler orchestrates job execution: validates, dispatches commands,
// collects results, and advances through multi-step jobs.
type Scheduler struct {
	client   *client.Client
	registry *registry.Registry

	mu      sync.Mutex
	running map[string]context.CancelFunc // active job ID → cancel
}

func New(c *client.Client, reg *registry.Registry) *Scheduler {
	return &Scheduler{
		client:   c,
		registry: reg,
		running:  make(map[string]context.CancelFunc),
	}
}

// Start begins pulling jobs from the requests stream (WorkQueue consumer).
func (s *Scheduler) Start(ctx context.Context) error {
	stream, err := s.client.GetStream("requests")
	if err != nil {
		return fmt.Errorf("getting requests stream: %w", err)
	}

	consumer, err := s.client.EnsureConsumer(stream, jetstream.ConsumerConfig{
		Durable:       "scheduler",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("creating scheduler consumer: %w", err)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				continue
			}
			for msg := range msgs.Messages() {
				s.handleRequest(ctx, msg)
			}
		}
	}()

	log.Info("Scheduler started, pulling from requests stream")
	return nil
}

// Enqueue validates, resolves targets, stores the job, and publishes to the
// requests stream. Returns the job with populated fields.
func (s *Scheduler) Enqueue(job models.Job) (models.Job, error) {
	// Resolve targets
	nodes, err := s.registry.Resolve(job.Target)
	if err != nil {
		return job, fmt.Errorf("resolving targets: %w", err)
	}

	job.Expected = registry.NodeIDs(nodes)
	job.Status = models.JobPending
	job.Step = 0
	job.Results = make(models.JobResults)

	// Store job
	job, err = s.client.CreateJob(job)
	if err != nil {
		return job, fmt.Errorf("creating job: %w", err)
	}

	// Publish to requests stream
	data, err := json.Marshal(job)
	if err != nil {
		return job, fmt.Errorf("encoding job: %w", err)
	}
	subject := fmt.Sprintf("request.%s", job.ID)
	if _, err := s.client.Publish(&nats.Msg{Subject: subject, Data: data}); err != nil {
		return job, fmt.Errorf("publishing request: %w", err)
	}
	log.Info("Job enqueued", "id", job.ID)

	return job, nil
}

func (s *Scheduler) handleRequest(ctx context.Context, msg jetstream.Msg) {
	var job models.Job
	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		log.Error("Failed to decode job request", "error", err)
		msg.Ack()
		return
	}
	log.Info("Scheduler picked up job", "id", job.ID)
	msg.Ack()

	execCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.running[job.ID] = cancel
	s.mu.Unlock()

	s.execute(execCtx, job)

	s.mu.Lock()
	delete(s.running, job.ID)
	s.mu.Unlock()
	cancel()
}

func (s *Scheduler) execute(ctx context.Context, job models.Job) {
	log.Info("Executing job", "id", job.ID, "tasks", len(job.Tasks), "nodes", len(job.Expected), "strategy", job.Strategy)

	job.Status = models.JobRunning
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to update job status", "id", job.ID, "error", err)
		return
	}

	// Track active nodes (for continue strategy, failed nodes are excluded)
	activeNodes := make([]string, len(job.Expected))
	copy(activeNodes, job.Expected)

	for i, task := range job.Tasks {
		if len(activeNodes) == 0 {
			log.Info("No active nodes remaining", "job", job.ID)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		job.Step = i
		if err := s.client.UpdateJob(job); err != nil {
			log.Error("Failed to update job step", "id", job.ID, "error", err)
		}

		log.Info("Dispatching task", "job", job.ID, "step", i, "backend", task.Backend, "action", task.Action, "nodes", len(activeNodes))

		// Publish command
		if err := s.client.PublishCommand(job.ID, i, job.Target, task); err != nil {
			log.Error("Failed to publish command", "job", job.ID, "step", i, "error", err)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		// Collect results for this step
		results, err := s.collectResults(ctx, job.ID, i, activeNodes)
		if err != nil {
			log.Error("Result collection failed", "job", job.ID, "step", i, "error", err)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		log.Info("Task step complete", "job", job.ID, "step", i, "results", len(results))

		// Store results
		s.storeResults(&job, i, results)

		// Check for failures
		for _, r := range results {
			if r.Status == models.ResultFailed {
				if job.Strategy == models.StrategyContinue {
					// Remove failed node from active set
					for j, id := range activeNodes {
						if id == r.NodeID {
							activeNodes = append(activeNodes[:j], activeNodes[j+1:]...)
							break
						}
					}
					log.Info("Node failed, continuing", "job", job.ID, "node", r.NodeID)
				} else {
					log.Info("Node failed, stopping", "job", job.ID, "node", r.NodeID)
					job.Status = models.JobFailed
					s.client.UpdateJob(job)
					return
				}
			}
		}

		if len(activeNodes) == 0 {
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}
	}

	job.Status = models.JobCompleted
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to mark job completed", "id", job.ID, "error", err)
	}
	log.Info("Job done", "id", job.ID, "status", job.Status)
}

// collectResults watches the results stream for a specific job+step and waits
// until all expected nodes have reported or timeout.
func (s *Scheduler) collectResults(ctx context.Context, jobID string, taskIndex int, expected []string) ([]models.TaskResult, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultTaskTimeout)
	defer cancel()

	// Create an ephemeral consumer on the results stream filtered to this job
	filterSubject := fmt.Sprintf("result.%s.>", jobID)

	stream, err := s.client.GetStream("results")
	if err != nil {
		return nil, fmt.Errorf("getting results stream: %w", err)
	}

	consumerName := fmt.Sprintf("sched-%s-%d", jobID, taskIndex)
	consumer, err := s.client.EnsureConsumer(stream, jetstream.ConsumerConfig{
		Name:           consumerName,
		FilterSubjects: []string{filterSubject},
		DeliverPolicy:  jetstream.DeliverLastPerSubjectPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return nil, fmt.Errorf("creating results consumer: %w", err)
	}
	defer s.client.DeleteConsumer(stream, consumerName)

	// Track which nodes have reported for this specific task index
	pending := make(map[string]bool, len(expected))
	for _, nodeID := range expected {
		pending[nodeID] = true
	}

	var results []models.TaskResult

	for len(pending) > 0 {
		msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return results, fmt.Errorf("timeout waiting for results from %d nodes", len(pending))
			}
			continue
		}

		for msg := range msgs.Messages() {
			var result models.TaskResult
			if err := json.Unmarshal(msg.Data(), &result); err != nil {
				log.Error("Failed to decode result", "error", err)
				msg.Ack()
				continue
			}

			// Only process results for our task index
			if result.TaskIndex != taskIndex {
				msg.Ack()
				continue
			}

			if pending[result.NodeID] {
				delete(pending, result.NodeID)
				results = append(results, result)
				log.Debug("Result received", "job", jobID, "step", taskIndex, "node", result.NodeID, "status", result.Status, "remaining", len(pending))
			}
			msg.Ack()
		}

		if ctx.Err() != nil {
			return results, fmt.Errorf("timeout waiting for results from %d nodes", len(pending))
		}
	}

	return results, nil
}

// storeResults persists per-node results for a step into the job.
func (s *Scheduler) storeResults(job *models.Job, step int, results []models.TaskResult) {
	stepKey := strconv.Itoa(step)
	if job.Results == nil {
		job.Results = make(models.JobResults)
	}
	job.Results[stepKey] = make(map[string]models.NodeResult)
	for _, r := range results {
		job.Results[stepKey][r.NodeID] = models.NodeResult{
			Status:   r.Status,
			Output:   r.Output,
			Error:    r.Error,
			Duration: r.Duration.String(),
		}
	}
	s.client.UpdateJob(*job)
}
