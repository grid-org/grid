package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
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

// Submit validates and begins executing a job. It runs asynchronously --
// the job is stored immediately and execution proceeds in a goroutine.
func (s *Scheduler) Submit(job models.Job) (models.Job, error) {
	// Resolve targets
	nodes, err := s.registry.Resolve(job.Target)
	if err != nil {
		return job, fmt.Errorf("resolving targets: %w", err)
	}

	job.Expected = registry.NodeIDs(nodes)
	job.Status = models.JobPending
	job.Step = 0

	// Store job
	job, err = s.client.CreateJob(job)
	if err != nil {
		return job, fmt.Errorf("creating job: %w", err)
	}

	// Execute asynchronously
	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.running[job.ID] = cancel
	s.mu.Unlock()

	go s.execute(ctx, job)

	return job, nil
}

func (s *Scheduler) execute(ctx context.Context, job models.Job) {
	defer func() {
		s.mu.Lock()
		delete(s.running, job.ID)
		s.mu.Unlock()
	}()

	log.Info("Executing job", "id", job.ID, "tasks", len(job.Tasks), "nodes", len(job.Expected))

	job.Status = models.JobRunning
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to update job status", "id", job.ID, "error", err)
		return
	}

	for i, task := range job.Tasks {
		job.Step = i
		if err := s.client.UpdateJob(job); err != nil {
			log.Error("Failed to update job step", "id", job.ID, "error", err)
		}

		log.Info("Dispatching task", "job", job.ID, "step", i, "backend", task.Backend, "action", task.Action)

		// Publish command
		if err := s.client.PublishCommand(job.ID, i, job.Target, task); err != nil {
			log.Error("Failed to publish command", "job", job.ID, "step", i, "error", err)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		// Collect results for this step
		results, err := s.collectResults(ctx, job.ID, i, job.Expected)
		if err != nil {
			log.Error("Result collection failed", "job", job.ID, "step", i, "error", err)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		// Check for failures
		for _, r := range results {
			if r.Status == models.ResultFailed {
				log.Error("Task failed on node", "job", job.ID, "step", i, "node", r.NodeID, "error", r.Error)
				job.Status = models.JobFailed
				s.client.UpdateJob(job)
				return
			}
		}

		log.Info("Task step complete", "job", job.ID, "step", i, "results", len(results))
	}

	job.Status = models.JobCompleted
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to mark job completed", "id", job.ID, "error", err)
	}
	log.Info("Job completed", "id", job.ID)
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
