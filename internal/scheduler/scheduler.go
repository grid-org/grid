package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const DefaultTaskTimeout = 5 * time.Minute

// ErrQueueFull is returned by Enqueue when the pending queue is at capacity.
var ErrQueueFull = errors.New("job queue is full")

// Scheduler orchestrates job execution: validates, dispatches commands,
// collects results, and advances through multi-step jobs.
type Scheduler struct {
	client   *client.Client
	registry *registry.Registry

	mu      sync.Mutex
	running map[string]context.CancelFunc // active job ID → cancel

	sem        chan struct{} // buffered semaphore limiting concurrent jobs
	pending    atomic.Int64  // count of enqueued but not-yet-running jobs
	maxPending int
}

func New(c *client.Client, reg *registry.Registry, cfg config.SchedulerConfig) *Scheduler {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 5
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 100
	}
	return &Scheduler{
		client:     c,
		registry:   reg,
		running:    make(map[string]context.CancelFunc),
		sem:        make(chan struct{}, cfg.MaxConcurrent),
		maxPending: cfg.MaxPending,
	}
}

// Start begins pulling jobs from the requests stream (WorkQueue consumer).
// Jobs execute concurrently up to the configured MaxConcurrent limit.
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
			// Acquire a concurrency slot (blocks if MaxConcurrent goroutines are active)
			select {
			case s.sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			// Fetch the next job from the work queue
			msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
			if err != nil {
				<-s.sem // release slot
				if ctx.Err() != nil {
					return
				}
				continue
			}

			gotMsg := false
			for msg := range msgs.Messages() {
				gotMsg = true
				s.pending.Add(-1) // no longer pending, now running
				go func(m jetstream.Msg) {
					defer func() { <-s.sem }() // release slot when done
					s.handleRequest(ctx, m)
				}(msg)
			}
			if !gotMsg {
				<-s.sem // release slot if no message received
			}
		}
	}()

	log.Info("Scheduler started, pulling from requests stream")
	return nil
}

// Enqueue validates, resolves targets, stores the job, and publishes to the
// requests stream. Returns ErrQueueFull if the pending queue is at capacity.
func (s *Scheduler) Enqueue(job models.Job) (models.Job, error) {
	// Admission control: reject if pending queue is full
	if s.pending.Load() >= int64(s.maxPending) {
		return job, ErrQueueFull
	}

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

	s.pending.Add(1)
	log.Info("Job enqueued", "id", job.ID, "pending", s.pending.Load())

	return job, nil
}

// Cancel stops a running job. Returns true if the job was running and cancelled.
func (s *Scheduler) Cancel(jobID string) (bool, error) {
	s.mu.Lock()
	cancel, ok := s.running[jobID]
	s.mu.Unlock()

	if !ok {
		// Not currently running — check if it's pending
		job, err := s.client.GetJob(jobID)
		if err != nil {
			return false, fmt.Errorf("job not found: %w", err)
		}
		if job.Status == models.JobPending {
			job.Status = models.JobCancelled
			return true, s.client.UpdateJob(job)
		}
		return false, nil
	}

	cancel()
	return true, nil
}

func (s *Scheduler) handleRequest(ctx context.Context, msg jetstream.Msg) {
	var job models.Job
	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		log.Error("Failed to decode job request", "error", err)
		msg.Ack()
		return
	}

	// Skip cancelled jobs
	if job.Status == models.JobCancelled {
		log.Info("Skipping cancelled job", "id", job.ID)
		msg.Ack()
		return
	}

	log.Info("Scheduler picked up job", "id", job.ID)
	msg.Ack()

	execCtx, cancel := context.WithCancel(ctx)

	// Apply job-level timeout if set
	if job.Timeout != "" {
		if d, err := time.ParseDuration(job.Timeout); err == nil {
			var timeoutCancel context.CancelFunc
			execCtx, timeoutCancel = context.WithTimeout(execCtx, d)
			defer timeoutCancel()
		} else {
			log.Warn("Invalid job timeout, using no limit", "job", job.ID, "timeout", job.Timeout)
		}
	}

	s.mu.Lock()
	s.running[job.ID] = cancel
	s.mu.Unlock()

	s.execute(execCtx, job)

	s.mu.Lock()
	delete(s.running, job.ID)
	s.mu.Unlock()
	cancel()
}

// ShouldExecute determines whether a task should run based on its condition
// and whether any previous step has failed.
func ShouldExecute(condition models.Condition, previousFailed bool) bool {
	switch condition {
	case models.ConditionOnSuccess:
		return !previousFailed
	case models.ConditionOnFailure:
		return previousFailed
	default: // "" or "always"
		return true
	}
}

// storeSkippedResults writes synthetic "skipped" results for all active nodes at a step.
func (s *Scheduler) storeSkippedResults(job *models.Job, step int, activeNodes []string) {
	stepKey := strconv.Itoa(step)
	if job.Results == nil {
		job.Results = make(models.JobResults)
	}
	job.Results[stepKey] = make(map[string]models.NodeResult)
	for _, nodeID := range activeNodes {
		job.Results[stepKey][nodeID] = models.NodeResult{
			Status:   models.ResultSkipped,
			Duration: "0s",
		}
	}
	s.client.UpdateJob(*job)
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

	// Track whether any prior step has failed (for conditional execution)
	stepFailed := false
	// Track whether fail-fast has been triggered (allows on_failure tasks to still run)
	failFastTriggered := false

	for i, task := range job.Tasks {
		// Check for cancellation
		if ctx.Err() != nil {
			log.Info("Job cancelled or timed out", "job", job.ID, "step", i)
			job.Status = models.JobCancelled
			s.client.UpdateJob(job)
			return
		}

		if len(activeNodes) == 0 {
			log.Info("No active nodes remaining", "job", job.ID)
			job.Status = models.JobFailed
			s.client.UpdateJob(job)
			return
		}

		// Evaluate task condition
		if failFastTriggered {
			// After fail-fast, only on_failure tasks may run
			if task.Condition != models.ConditionOnFailure {
				log.Info("Skipping task (fail-fast triggered)", "job", job.ID, "step", i, "condition", task.Condition)
				s.storeSkippedResults(&job, i, activeNodes)
				continue
			}
		} else if !ShouldExecute(task.Condition, stepFailed) {
			log.Info("Skipping task (condition not met)", "job", job.ID, "step", i, "condition", task.Condition)
			s.storeSkippedResults(&job, i, activeNodes)
			continue
		}

		job.Step = i
		if err := s.client.UpdateJob(job); err != nil {
			log.Error("Failed to update job step", "id", job.ID, "error", err)
		}

		log.Info("Dispatching task", "job", job.ID, "step", i, "backend", task.Backend, "action", task.Action, "nodes", len(activeNodes))

		// Determine task timeout
		taskTimeout := DefaultTaskTimeout
		if task.Timeout != "" {
			if d, err := time.ParseDuration(task.Timeout); err == nil {
				taskTimeout = d
			}
		}

		// Retry loop: attempt the task up to 1 + MaxRetries times
		maxAttempts := 1 + task.MaxRetries
		var results []models.TaskResult
		var lastErr error

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if attempt > 1 {
				// Exponential backoff: 2^(attempt-2) seconds, capped at 30s
				backoff := time.Duration(1<<uint(attempt-2)) * time.Second
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				log.Info("Retrying task", "job", job.ID, "step", i, "attempt", attempt, "backoff", backoff)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					job.Status = models.JobCancelled
					s.client.UpdateJob(job)
					return
				}

				// Re-dispatch command for retry
				if err := s.client.PublishCommand(job.ID, i, job.Target, task); err != nil {
					log.Error("Failed to publish retry command", "job", job.ID, "step", i, "error", err)
					job.Status = models.JobFailed
					s.client.UpdateJob(job)
					return
				}
			} else {
				// First attempt: publish command
				if err := s.client.PublishCommand(job.ID, i, job.Target, task); err != nil {
					log.Error("Failed to publish command", "job", job.ID, "step", i, "error", err)
					job.Status = models.JobFailed
					s.client.UpdateJob(job)
					return
				}
			}

			// Collect results for this step
			var err error
			results, err = s.collectResults(ctx, job.ID, i, activeNodes, taskTimeout)
			if err != nil {
				if ctx.Err() != nil {
					log.Info("Job cancelled during result collection", "job", job.ID, "step", i)
					job.Status = models.JobCancelled
				} else {
					log.Error("Result collection failed", "job", job.ID, "step", i, "error", err)
					job.Status = models.JobFailed
				}
				s.client.UpdateJob(job)
				return
			}

			// Check if any nodes failed
			anyFailed := false
			for _, r := range results {
				if r.Status == models.ResultFailed {
					anyFailed = true
					break
				}
			}

			if !anyFailed {
				// All succeeded, store and move on
				s.storeResults(&job, i, results, attempt)
				lastErr = nil
				break
			}

			// There were failures
			if attempt < maxAttempts {
				lastErr = fmt.Errorf("failures on attempt %d", attempt)
				continue // retry
			}

			// Final attempt with failures — store and apply strategy
			s.storeResults(&job, i, results, attempt)
			lastErr = fmt.Errorf("failures after %d attempts", maxAttempts)
		}

		log.Info("Task step complete", "job", job.ID, "step", i, "results", len(results))

		if lastErr != nil {
			// Mark that a step has failed (for conditional execution)
			stepFailed = true

			// Apply failure strategy on final failure
			for _, r := range results {
				if r.Status == models.ResultFailed {
					if job.Strategy == models.StrategyContinue {
						for j, id := range activeNodes {
							if id == r.NodeID {
								activeNodes = append(activeNodes[:j], activeNodes[j+1:]...)
								break
							}
						}
						log.Info("Node failed, continuing", "job", job.ID, "node", r.NodeID)
					} else {
						log.Info("Node failed, fail-fast triggered", "job", job.ID, "node", r.NodeID)
						failFastTriggered = true
					}
				}
			}

			if len(activeNodes) == 0 {
				job.Status = models.JobFailed
				s.client.UpdateJob(job)
				return
			}
		}
	}

	if failFastTriggered {
		job.Status = models.JobFailed
	} else {
		job.Status = models.JobCompleted
	}
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to mark job done", "id", job.ID, "error", err)
	}
	log.Info("Job done", "id", job.ID, "status", job.Status)
}

// collectResults watches the results stream for a specific job+step and waits
// until all expected nodes have reported or timeout.
func (s *Scheduler) collectResults(ctx context.Context, jobID string, taskIndex int, expected []string, timeout time.Duration) ([]models.TaskResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
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
func (s *Scheduler) storeResults(job *models.Job, step int, results []models.TaskResult, attempts int) {
	stepKey := strconv.Itoa(step)
	if job.Results == nil {
		job.Results = make(models.JobResults)
	}
	job.Results[stepKey] = make(map[string]models.NodeResult)
	for _, r := range results {
		nr := models.NodeResult{
			Status:   r.Status,
			Output:   r.Output,
			Error:    r.Error,
			Duration: r.Duration.String(),
		}
		if attempts > 1 {
			nr.Attempts = attempts
		}
		job.Results[stepKey][r.NodeID] = nr
	}
	s.client.UpdateJob(*job)
}
