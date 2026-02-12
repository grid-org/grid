package scheduler_test

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/scheduler"
	"github.com/grid-org/grid/internal/testutil"
	"github.com/nats-io/nats.go/jetstream"
)

// mockWorkerBehavior controls how a mock worker responds to commands.
type mockWorkerBehavior int

const (
	behaviorSucceed mockWorkerBehavior = iota
	behaviorFail
	behaviorSlow // succeed after a delay
)

// startMockWorkers creates lightweight goroutines that consume commands and
// publish results. They filter on cmd.all.> to receive broadcast commands.
// Returns a cancel function that stops all workers.
func startMockWorkers(t *testing.T, env *testutil.TestEnv, nodeIDs []string, behavior mockWorkerBehavior) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := env.Client.GetStream("commands")
	if err != nil {
		t.Fatalf("getting commands stream: %v", err)
	}

	for _, id := range nodeIDs {
		nodeID := id
		consumerName := fmt.Sprintf("mock-%s", nodeID)
		consumer, err := env.Client.EnsureConsumer(stream, jetstream.ConsumerConfig{
			Durable:        consumerName,
			FilterSubjects: []string{"cmd.>"},
			DeliverPolicy:  jetstream.DeliverNewPolicy,
			AckPolicy:      jetstream.AckExplicitPolicy,
		})
		if err != nil {
			t.Fatalf("creating mock consumer for %s: %v", nodeID, err)
		}

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(500*time.Millisecond))
				if err != nil {
					continue
				}

				for msg := range msgs.Messages() {
					jobID := msg.Headers().Get("job-id")
					taskIndex, _ := strconv.Atoi(msg.Headers().Get("task-index"))

					status := models.ResultSuccess
					errMsg := ""

					switch behavior {
					case behaviorFail:
						status = models.ResultFailed
						errMsg = "mock failure"
					case behaviorSlow:
						select {
						case <-time.After(3 * time.Second):
						case <-ctx.Done():
							msg.Ack()
							return
						}
					}

					result := models.TaskResult{
						JobID:     jobID,
						TaskIndex: taskIndex,
						NodeID:    nodeID,
						Status:    status,
						Output:    "mock output",
						Error:     errMsg,
						Duration:  50 * time.Millisecond,
						Timestamp: time.Now(),
					}
					env.Client.PublishResult(result)
					msg.Ack()
				}
			}
		}()
	}

	return cancel
}

// startMixedMockWorkers creates workers where specific nodes fail.
func startMixedMockWorkers(t *testing.T, env *testutil.TestEnv, succeedIDs, failIDs []string) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	stream, err := env.Client.GetStream("commands")
	if err != nil {
		t.Fatalf("getting commands stream: %v", err)
	}

	failSet := make(map[string]bool)
	for _, id := range failIDs {
		failSet[id] = true
	}

	allIDs := append(succeedIDs, failIDs...)
	for _, id := range allIDs {
		nodeID := id
		consumerName := fmt.Sprintf("mock-%s", nodeID)
		consumer, err := env.Client.EnsureConsumer(stream, jetstream.ConsumerConfig{
			Durable:        consumerName,
			FilterSubjects: []string{"cmd.>"},
			DeliverPolicy:  jetstream.DeliverNewPolicy,
			AckPolicy:      jetstream.AckExplicitPolicy,
		})
		if err != nil {
			t.Fatalf("creating mock consumer for %s: %v", nodeID, err)
		}

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(500*time.Millisecond))
				if err != nil {
					continue
				}

				for msg := range msgs.Messages() {
					jobID := msg.Headers().Get("job-id")
					taskIndex, _ := strconv.Atoi(msg.Headers().Get("task-index"))

					status := models.ResultSuccess
					errMsg := ""
					if failSet[nodeID] {
						status = models.ResultFailed
						errMsg = "mock failure"
					}

					result := models.TaskResult{
						JobID:     jobID,
						TaskIndex: taskIndex,
						NodeID:    nodeID,
						Status:    status,
						Output:    "mock output",
						Error:     errMsg,
						Duration:  50 * time.Millisecond,
						Timestamp: time.Now(),
					}
					env.Client.PublishResult(result)
					msg.Ack()
				}
			}
		}()
	}

	return cancel
}

func setupScheduler(t *testing.T, env *testutil.TestEnv, nodeIDs []string, groups ...string) *scheduler.Scheduler {
	t.Helper()
	for _, id := range nodeIDs {
		env.RegisterNodes(t, testutil.OnlineNode(id, groups...))
	}
	reg := registry.New(env.Client)
	return scheduler.New(env.Client, reg)
}

// --- Tests ---

func TestEnqueue(t *testing.T) {
	env := testutil.NewTestEnv(t)
	sched := setupScheduler(t, env, []string{"n1", "n2"}, "web")

	job := models.Job{
		ID:     "enqueue-1",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
	}

	enqueued, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	if enqueued.Status != models.JobPending {
		t.Errorf("Status = %q, want pending", enqueued.Status)
	}
	if len(enqueued.Expected) != 2 {
		t.Errorf("Expected len = %d, want 2", len(enqueued.Expected))
	}

	// Verify stored in KV
	stored, err := env.Client.GetJob("enqueue-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if stored.Status != models.JobPending {
		t.Errorf("stored Status = %q, want pending", stored.Status)
	}
}

func TestEnqueue_InvalidTarget(t *testing.T) {
	env := testutil.NewTestEnv(t)
	sched := setupScheduler(t, env, []string{"n1"}, "web")

	job := models.Job{
		ID:     "enqueue-bad",
		Target: models.Target{Scope: "group", Value: "nonexistent"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
	}

	_, err := sched.Enqueue(job)
	if err == nil {
		t.Fatal("expected error for unresolvable target")
	}
}

func TestExecute_SingleTask(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1", "n2"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stopWorkers := startMockWorkers(t, env, nodes, behaviorSucceed)
	defer stopWorkers()

	job := models.Job{
		ID:     "exec-single",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-single")
		return j.Status == models.JobCompleted
	}, "job should complete")

	got, _ := env.Client.GetJob("exec-single")
	if len(got.Results) != 1 {
		t.Fatalf("Results has %d steps, want 1", len(got.Results))
	}
	if len(got.Results["0"]) != 2 {
		t.Errorf("step 0 has %d nodes, want 2", len(got.Results["0"]))
	}
}

func TestExecute_MultiStep(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1", "n2"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stopWorkers := startMockWorkers(t, env, nodes, behaviorSucceed)
	defer stopWorkers()

	job := models.Job{
		ID:     "exec-multi",
		Target: models.Target{Scope: "all"},
		Tasks: []models.Task{
			{Backend: "test", Action: "succeed"},
			{Backend: "test", Action: "succeed"},
			{Backend: "test", Action: "succeed"},
		},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 15*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-multi")
		return j.Status == models.JobCompleted
	}, "job should complete")

	got, _ := env.Client.GetJob("exec-multi")
	if len(got.Results) != 3 {
		t.Errorf("Results has %d steps, want 3", len(got.Results))
	}
	for step := 0; step < 3; step++ {
		key := strconv.Itoa(step)
		if len(got.Results[key]) != 2 {
			t.Errorf("step %d has %d nodes, want 2", step, len(got.Results[key]))
		}
	}
}

func TestExecute_FailFast(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1", "n2"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// n1 succeeds, n2 fails
	stopWorkers := startMixedMockWorkers(t, env, []string{"n1"}, []string{"n2"})
	defer stopWorkers()

	job := models.Job{
		ID:       "exec-failfast",
		Target:   models.Target{Scope: "all"},
		Strategy: models.StrategyFailFast,
		Tasks: []models.Task{
			{Backend: "test", Action: "succeed"},
			{Backend: "test", Action: "succeed"}, // should not execute
		},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-failfast")
		return j.Status == models.JobFailed
	}, "job should fail")

	got, _ := env.Client.GetJob("exec-failfast")
	if got.Status != models.JobFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	// Step 1 should not have results
	if len(got.Results["1"]) > 0 {
		t.Errorf("step 1 should not have results, has %d", len(got.Results["1"]))
	}
}

func TestExecute_Continue(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1", "n2", "n3"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// n1, n2 succeed; n3 fails
	stopWorkers := startMixedMockWorkers(t, env, []string{"n1", "n2"}, []string{"n3"})
	defer stopWorkers()

	job := models.Job{
		ID:       "exec-continue",
		Target:   models.Target{Scope: "all"},
		Strategy: models.StrategyContinue,
		Tasks: []models.Task{
			{Backend: "test", Action: "succeed"},
			{Backend: "test", Action: "succeed"},
		},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 15*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-continue")
		return j.Status == models.JobCompleted || j.Status == models.JobFailed
	}, "job should finish")

	got, _ := env.Client.GetJob("exec-continue")
	if got.Status != models.JobCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	// Step 0 should have 3 results (one failed)
	if len(got.Results["0"]) != 3 {
		t.Errorf("step 0 has %d nodes, want 3", len(got.Results["0"]))
	}
	// Step 1 should have 2 results (failed node excluded)
	if len(got.Results["1"]) != 2 {
		t.Errorf("step 1 has %d nodes, want 2", len(got.Results["1"]))
	}
}

func TestCancel_Pending(t *testing.T) {
	env := testutil.NewTestEnv(t)
	sched := setupScheduler(t, env, []string{"n1"}, "web")

	// Don't start the scheduler — job stays pending
	job := models.Job{
		ID:     "cancel-pending",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	cancelled, err := sched.Cancel("cancel-pending")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cancelled {
		t.Error("Cancel should return true for pending job")
	}

	got, _ := env.Client.GetJob("cancel-pending")
	if got.Status != models.JobCancelled {
		t.Errorf("Status = %q, want cancelled", got.Status)
	}
}

func TestCancel_Running(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Use slow workers so job stays running long enough to cancel
	stopWorkers := startMockWorkers(t, env, nodes, behaviorSlow)
	defer stopWorkers()

	job := models.Job{
		ID:     "cancel-running",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "sleep", Params: map[string]string{"duration": "30s"}}},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	// Wait for job to be picked up
	testutil.WaitFor(t, 5*time.Second, func() bool {
		j, _ := env.Client.GetJob("cancel-running")
		return j.Status == models.JobRunning
	}, "job should start running")

	cancelled, err := sched.Cancel("cancel-running")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !cancelled {
		t.Error("Cancel should return true for running job")
	}

	testutil.WaitFor(t, 5*time.Second, func() bool {
		j, _ := env.Client.GetJob("cancel-running")
		return j.Status == models.JobCancelled
	}, "job should become cancelled")
}

func TestExecute_TaskTimeout(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Workers that never respond (no mock workers started)
	// The task timeout should expire

	job := models.Job{
		ID:     "exec-timeout",
		Target: models.Target{Scope: "all"},
		Tasks: []models.Task{{
			Backend: "test",
			Action:  "succeed",
			Timeout: "2s", // short timeout
		}},
	}

	_, err := sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-timeout")
		return j.Status == models.JobFailed || j.Status == models.JobCancelled
	}, "job should fail from timeout")
}

func TestExecute_Retry(t *testing.T) {
	env := testutil.NewTestEnv(t)
	nodes := []string{"n1"}
	sched := setupScheduler(t, env, nodes, "web")

	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	if err := sched.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Start workers that track attempt count and succeed on attempt 3
	stream, err := env.Client.GetStream("commands")
	if err != nil {
		t.Fatalf("getting commands stream: %v", err)
	}

	var mu sync.Mutex
	attemptCount := 0

	consumer, err := env.Client.EnsureConsumer(stream, jetstream.ConsumerConfig{
		Durable:        "mock-retry-n1",
		FilterSubjects: []string{"cmd.>"},
		DeliverPolicy:  jetstream.DeliverNewPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
	})
	if err != nil {
		t.Fatalf("creating retry consumer: %v", err)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			default:
			}

			msgs, err := consumer.Fetch(1, jetstream.FetchMaxWait(500*time.Millisecond))
			if err != nil {
				continue
			}

			for msg := range msgs.Messages() {
				jobID := msg.Headers().Get("job-id")
				taskIndex, _ := strconv.Atoi(msg.Headers().Get("task-index"))

				mu.Lock()
				attemptCount++
				attempt := attemptCount
				mu.Unlock()

				status := models.ResultFailed
				errMsg := "not yet"
				if attempt >= 3 {
					status = models.ResultSuccess
					errMsg = ""
				}

				result := models.TaskResult{
					JobID:     jobID,
					TaskIndex: taskIndex,
					NodeID:    "n1",
					Status:    status,
					Output:    fmt.Sprintf("attempt %d", attempt),
					Error:     errMsg,
					Duration:  50 * time.Millisecond,
					Timestamp: time.Now(),
				}
				env.Client.PublishResult(result)
				msg.Ack()
			}
		}
	}()

	job := models.Job{
		ID:     "exec-retry",
		Target: models.Target{Scope: "all"},
		Tasks: []models.Task{{
			Backend:    "test",
			Action:     "succeed",
			MaxRetries: 3,
			Timeout:    "30s",
		}},
	}

	_, err = sched.Enqueue(job)
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	testutil.WaitFor(t, 30*time.Second, func() bool {
		j, _ := env.Client.GetJob("exec-retry")
		return j.Status == models.JobCompleted || j.Status == models.JobFailed
	}, "job should finish")

	got, _ := env.Client.GetJob("exec-retry")
	if got.Status != models.JobCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}

	// Should have taken 3 attempts
	nr := got.Results["0"]["n1"]
	if nr.Attempts < 2 {
		t.Errorf("Attempts = %d, want >= 2", nr.Attempts)
	}
}
