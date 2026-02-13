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

// execState tracks mutable execution state across a job's phase tree.
type execState struct {
	activeNodes       []string
	flatIndex         int
	stepFailed        bool
	failFastTriggered bool
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

func (s *Scheduler) execute(ctx context.Context, job models.Job) {
	leafCount := models.PhaseLeafCount(job.Tasks)
	log.Info("Executing job", "id", job.ID, "phases", len(job.Tasks), "leaves", leafCount, "nodes", len(job.Expected), "strategy", job.Strategy)

	job.Status = models.JobRunning
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to update job status", "id", job.ID, "error", err)
		return
	}

	state := &execState{
		activeNodes: make([]string, len(job.Expected)),
	}
	copy(state.activeNodes, job.Expected)

	s.executePhases(ctx, &job, job.Tasks, state)

	// If the job was already set to a terminal status by a sub-method, respect it
	if job.Status == models.JobCancelled || job.Status == models.JobFailed {
		log.Info("Job done", "id", job.ID, "status", job.Status)
		return
	}

	if ctx.Err() != nil {
		job.Status = models.JobCancelled
		s.client.UpdateJob(job)
		log.Info("Job done", "id", job.ID, "status", job.Status)
		return
	}

	if state.failFastTriggered {
		job.Status = models.JobFailed
	} else {
		job.Status = models.JobCompleted
	}
	if err := s.client.UpdateJob(job); err != nil {
		log.Error("Failed to mark job done", "id", job.ID, "error", err)
	}
	log.Info("Job done", "id", job.ID, "status", job.Status)
}

// executePhases walks a list of phases (barrier-synchronized at this level).
func (s *Scheduler) executePhases(ctx context.Context, job *models.Job, phases []models.Phase, state *execState) {
	for _, phase := range phases {
		if ctx.Err() != nil {
			log.Info("Job cancelled or timed out", "job", job.ID, "flatIndex", state.flatIndex)
			job.Status = models.JobCancelled
			s.client.UpdateJob(*job)
			return
		}

		if len(state.activeNodes) == 0 {
			log.Info("No active nodes remaining", "job", job.ID)
			job.Status = models.JobFailed
			s.client.UpdateJob(*job)
			return
		}

		// Evaluate phase condition
		if state.failFastTriggered {
			if phase.Condition != models.ConditionOnFailure {
				log.Info("Skipping phase (fail-fast triggered)", "job", job.ID, "flatIndex", state.flatIndex, "condition", phase.Condition)
				s.storeSkippedPhase(job, phase, state)
				continue
			}
		} else if !ShouldExecute(phase.Condition, state.stepFailed) {
			log.Info("Skipping phase (condition not met)", "job", job.ID, "flatIndex", state.flatIndex, "condition", phase.Condition)
			s.storeSkippedPhase(job, phase, state)
			continue
		}

		if phase.IsLeaf() {
			s.executeBarrierStep(ctx, job, phase, state)
		} else {
			s.executePipeline(ctx, job, phase, state)
		}

		// Check if job was cancelled/failed during execution
		if job.Status == models.JobCancelled || job.Status == models.JobFailed {
			return
		}
	}
}

// storeSkippedPhase writes skipped results for all leaves in a phase (recursive for pipelines).
func (s *Scheduler) storeSkippedPhase(job *models.Job, phase models.Phase, state *execState) {
	if phase.IsLeaf() {
		s.storeSkippedResults(job, state.flatIndex, state.activeNodes)
		state.flatIndex++
	} else {
		for _, sub := range phase.Tasks {
			s.storeSkippedPhase(job, sub, state)
		}
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

// executeBarrierStep dispatches a leaf phase to all active nodes via broadcast,
// collects results, and applies the failure strategy. This is the standard
// barrier-synchronized execution mode.
func (s *Scheduler) executeBarrierStep(ctx context.Context, job *models.Job, phase models.Phase, state *execState) {
	task := phase.ToTask()
	stepIndex := state.flatIndex

	job.Step = stepIndex
	if err := s.client.UpdateJob(*job); err != nil {
		log.Error("Failed to update job step", "id", job.ID, "error", err)
	}

	log.Info("Dispatching barrier task", "job", job.ID, "step", stepIndex, "backend", task.Backend, "action", task.Action, "nodes", len(state.activeNodes))

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
			log.Info("Retrying task", "job", job.ID, "step", stepIndex, "attempt", attempt, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				job.Status = models.JobCancelled
				s.client.UpdateJob(*job)
				return
			}

			// Re-dispatch command for retry
			if err := s.client.PublishCommand(job.ID, stepIndex, job.Target, task); err != nil {
				log.Error("Failed to publish retry command", "job", job.ID, "step", stepIndex, "error", err)
				job.Status = models.JobFailed
				s.client.UpdateJob(*job)
				return
			}
		} else {
			// First attempt: publish command
			if err := s.client.PublishCommand(job.ID, stepIndex, job.Target, task); err != nil {
				log.Error("Failed to publish command", "job", job.ID, "step", stepIndex, "error", err)
				job.Status = models.JobFailed
				s.client.UpdateJob(*job)
				return
			}
		}

		// Collect results for this step
		var err error
		results, err = s.collectResults(ctx, job.ID, stepIndex, state.activeNodes, taskTimeout)
		if err != nil {
			if ctx.Err() != nil {
				log.Info("Job cancelled during result collection", "job", job.ID, "step", stepIndex)
				job.Status = models.JobCancelled
			} else {
				log.Error("Result collection failed", "job", job.ID, "step", stepIndex, "error", err)
				job.Status = models.JobFailed
			}
			s.client.UpdateJob(*job)
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
			s.storeResults(job, stepIndex, results, attempt)
			lastErr = nil
			break
		}

		// There were failures
		if attempt < maxAttempts {
			lastErr = fmt.Errorf("failures on attempt %d", attempt)
			continue // retry
		}

		// Final attempt with failures — store and apply strategy
		s.storeResults(job, stepIndex, results, attempt)
		lastErr = fmt.Errorf("failures after %d attempts", maxAttempts)
	}

	log.Info("Barrier step complete", "job", job.ID, "step", stepIndex, "results", len(results))
	state.flatIndex++

	if lastErr != nil {
		state.stepFailed = true

		// Apply failure strategy on final failure
		for _, r := range results {
			if r.Status == models.ResultFailed {
				if job.Strategy == models.StrategyContinue {
					for j, id := range state.activeNodes {
						if id == r.NodeID {
							state.activeNodes = append(state.activeNodes[:j], state.activeNodes[j+1:]...)
							break
						}
					}
					log.Info("Node failed, continuing", "job", job.ID, "node", r.NodeID)
				} else {
					log.Info("Node failed, fail-fast triggered", "job", job.ID, "node", r.NodeID)
					state.failFastTriggered = true
				}
			}
		}

		if len(state.activeNodes) == 0 {
			job.Status = models.JobFailed
			s.client.UpdateJob(*job)
			return
		}
	}
}

// executePipeline dispatches a pipeline phase (branch) to each node independently.
// Each node advances through sub-steps at its own pace without waiting for other nodes.
func (s *Scheduler) executePipeline(ctx context.Context, job *models.Job, phase models.Phase, state *execState) {
	subPhases := phase.Tasks
	startIndex := state.flatIndex

	log.Info("Dispatching pipeline", "job", job.ID, "startIndex", startIndex, "subSteps", len(subPhases), "nodes", len(state.activeNodes))

	// Per-node progress tracking
	type nodeProgress struct {
		currentStep int
		retries     int
		done        bool
		failed      bool
	}
	progress := make(map[string]*nodeProgress, len(state.activeNodes))
	for _, nodeID := range state.activeNodes {
		progress[nodeID] = &nodeProgress{}
	}

	// Create ephemeral consumer for pipeline results BEFORE dispatching
	stream, err := s.client.GetStream("results")
	if err != nil {
		log.Error("Failed to get results stream for pipeline", "job", job.ID, "error", err)
		job.Status = models.JobFailed
		s.client.UpdateJob(*job)
		return
	}

	filterSubject := fmt.Sprintf("result.%s.>", job.ID)
	consumerName := fmt.Sprintf("sched-pipe-%s-%d", job.ID, startIndex)
	consumer, err := s.client.EnsureConsumer(stream, jetstream.ConsumerConfig{
		Name:           consumerName,
		FilterSubjects: []string{filterSubject},
		DeliverPolicy:  jetstream.DeliverLastPerSubjectPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	if err != nil {
		log.Error("Failed to create pipeline consumer", "job", job.ID, "error", err)
		job.Status = models.JobFailed
		s.client.UpdateJob(*job)
		return
	}
	defer s.client.DeleteConsumer(stream, consumerName)

	// Dispatch first sub-step to all active nodes
	firstTask := subPhases[0].ToTask()
	for _, nodeID := range state.activeNodes {
		if err := s.client.PublishCommandToNode(job.ID, startIndex, nodeID, firstTask); err != nil {
			log.Error("Failed to dispatch pipeline step to node", "job", job.ID, "node", nodeID, "error", err)
			job.Status = models.JobFailed
			s.client.UpdateJob(*job)
			return
		}
	}

	// Determine timeout for the whole pipeline (use max of sub-step timeouts, or default)
	pipelineTimeout := DefaultTaskTimeout
	for _, sub := range subPhases {
		if sub.Timeout != "" {
			if d, err := time.ParseDuration(sub.Timeout); err == nil && d > pipelineTimeout {
				pipelineTimeout = d
			}
		}
	}
	// Scale timeout by number of sub-steps
	pipelineTimeout = pipelineTimeout * time.Duration(len(subPhases))

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, pipelineTimeout)
	defer timeoutCancel()

	// Result collection loop
	for {
		// Check if all nodes are done
		allDone := true
		for _, np := range progress {
			if !np.done {
				allDone = false
				break
			}
		}
		if allDone {
			break
		}

		if timeoutCtx.Err() != nil {
			if ctx.Err() != nil {
				log.Info("Job cancelled during pipeline", "job", job.ID)
				job.Status = models.JobCancelled
			} else {
				log.Error("Pipeline timed out", "job", job.ID)
				job.Status = models.JobFailed
			}
			s.client.UpdateJob(*job)
			return
		}

		msgs, fetchErr := consumer.Fetch(1, jetstream.FetchMaxWait(2*time.Second))
		if fetchErr != nil {
			if timeoutCtx.Err() != nil {
				if ctx.Err() != nil {
					job.Status = models.JobCancelled
				} else {
					job.Status = models.JobFailed
				}
				s.client.UpdateJob(*job)
				return
			}
			continue
		}

		for msg := range msgs.Messages() {
			var result models.TaskResult
			if err := json.Unmarshal(msg.Data(), &result); err != nil {
				log.Error("Failed to decode pipeline result", "error", err)
				msg.Ack()
				continue
			}

			np, ok := progress[result.NodeID]
			if !ok || np.done {
				msg.Ack()
				continue
			}

			expectedGlobalIndex := startIndex + np.currentStep
			if result.TaskIndex != expectedGlobalIndex {
				msg.Ack()
				continue
			}

			if result.Status == models.ResultFailed {
				subPhase := subPhases[np.currentStep]
				maxRetries := subPhase.MaxRetries

				if np.retries < maxRetries {
					np.retries++
					// Exponential backoff
					backoff := time.Duration(1<<uint(np.retries-1)) * time.Second
					if backoff > 30*time.Second {
						backoff = 30 * time.Second
					}
					log.Info("Retrying pipeline sub-step", "job", job.ID, "node", result.NodeID, "step", np.currentStep, "retry", np.retries, "backoff", backoff)

					go func(nodeID string, globalIdx int, task models.Task, delay time.Duration) {
						select {
						case <-time.After(delay):
						case <-ctx.Done():
							return
						}
						s.client.PublishCommandToNode(job.ID, globalIdx, nodeID, task)
					}(result.NodeID, expectedGlobalIndex, subPhase.ToTask(), backoff)

					msg.Ack()
					continue
				}

				// Sub-step failed after all retries
				s.storeSingleResult(job, expectedGlobalIndex, result, np.retries+1)
				state.stepFailed = true

				if job.Strategy == models.StrategyContinue {
					log.Info("Pipeline node failed, continuing", "job", job.ID, "node", result.NodeID, "step", np.currentStep)
					np.done = true
					np.failed = true
					// Store skipped results for remaining sub-steps
					for si := np.currentStep + 1; si < len(subPhases); si++ {
						globalIdx := startIndex + si
						s.storeSingleSkipped(job, globalIdx, result.NodeID)
					}
				} else {
					log.Info("Pipeline node failed, fail-fast triggered", "job", job.ID, "node", result.NodeID, "step", np.currentStep)
					state.failFastTriggered = true
					np.done = true
					np.failed = true
					// Mark all other nodes as done too
					for nid, other := range progress {
						if !other.done {
							other.done = true
							other.failed = true
							// Store skipped for their remaining sub-steps
							for si := other.currentStep; si < len(subPhases); si++ {
								globalIdx := startIndex + si
								s.storeSingleSkipped(job, globalIdx, nid)
							}
						}
					}
				}

				msg.Ack()
				continue
			}

			// Success — store result and advance
			s.storeSingleResult(job, expectedGlobalIndex, result, np.retries+1)
			np.retries = 0 // reset for next sub-step
			np.currentStep++

			if np.currentStep >= len(subPhases) {
				// Node completed all sub-steps
				np.done = true
				log.Debug("Pipeline node completed all sub-steps", "job", job.ID, "node", result.NodeID)
			} else {
				// Dispatch next sub-step to this node
				nextTask := subPhases[np.currentStep].ToTask()
				nextGlobalIdx := startIndex + np.currentStep
				if err := s.client.PublishCommandToNode(job.ID, nextGlobalIdx, result.NodeID, nextTask); err != nil {
					log.Error("Failed to dispatch next pipeline step", "job", job.ID, "node", result.NodeID, "error", err)
					np.done = true
					np.failed = true
				}
			}

			msg.Ack()
		}
	}

	// Advance flat index past all sub-steps
	state.flatIndex += len(subPhases)

	// Remove failed nodes from activeNodes (for continue strategy)
	if job.Strategy == models.StrategyContinue {
		for nodeID, np := range progress {
			if np.failed {
				for j, id := range state.activeNodes {
					if id == nodeID {
						state.activeNodes = append(state.activeNodes[:j], state.activeNodes[j+1:]...)
						break
					}
				}
			}
		}
		if len(state.activeNodes) == 0 {
			job.Status = models.JobFailed
			s.client.UpdateJob(*job)
			return
		}
	}

	log.Info("Pipeline complete", "job", job.ID, "startIndex", startIndex, "subSteps", len(subPhases))
}

// collectResults watches the results stream for a specific job+step and waits
// until all expected nodes have reported or timeout.
func (s *Scheduler) collectResults(ctx context.Context, jobID string, taskIndex int, expected []string, timeout time.Duration) ([]models.TaskResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create an ephemeral consumer on the results stream filtered to this job+step
	filterSubject := fmt.Sprintf("result.%s.%d.>", jobID, taskIndex)

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

// storeSingleResult persists a single node's result for a step.
func (s *Scheduler) storeSingleResult(job *models.Job, step int, result models.TaskResult, attempts int) {
	stepKey := strconv.Itoa(step)
	if job.Results == nil {
		job.Results = make(models.JobResults)
	}
	if job.Results[stepKey] == nil {
		job.Results[stepKey] = make(map[string]models.NodeResult)
	}
	nr := models.NodeResult{
		Status:   result.Status,
		Output:   result.Output,
		Error:    result.Error,
		Duration: result.Duration.String(),
	}
	if attempts > 1 {
		nr.Attempts = attempts
	}
	job.Results[stepKey][result.NodeID] = nr
	s.client.UpdateJob(*job)
}

// storeSingleSkipped writes a skipped result for a single node at a step.
func (s *Scheduler) storeSingleSkipped(job *models.Job, step int, nodeID string) {
	stepKey := strconv.Itoa(step)
	if job.Results == nil {
		job.Results = make(models.JobResults)
	}
	if job.Results[stepKey] == nil {
		job.Results[stepKey] = make(map[string]models.NodeResult)
	}
	job.Results[stepKey][nodeID] = models.NodeResult{
		Status:   models.ResultSkipped,
		Duration: "0s",
	}
	s.client.UpdateJob(*job)
}
