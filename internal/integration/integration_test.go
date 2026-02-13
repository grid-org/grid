package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/scheduler"
	"github.com/grid-org/grid/internal/testutil"
	"github.com/grid-org/grid/internal/worker/backends"
	"github.com/nats-io/nats.go/jetstream"
)

// testWorker is a lightweight worker for integration tests. It connects via TCP
// (not IPC) to the embedded NATS server, registers, and starts consuming commands.
type testWorker struct {
	client   *client.Client
	backends *backends.Backends
	nodeID   string
	groups   []string
	stopHB   context.CancelFunc
	context  jetstream.ConsumeContext
}

func startTestWorker(t *testing.T, env *testutil.TestEnv, nodeID string, groups []string) *testWorker {
	t.Helper()

	cfg := &config.Config{
		NATS: config.NATSConfig{
			Name: nodeID,
			Client: config.ClientConfig{
				URLS: env.Config.NATS.Client.URLS,
			},
		},
		Worker: config.WorkerConfig{
			Groups: groups,
		},
	}

	c, err := client.New(cfg, nil)
	if err != nil {
		t.Fatalf("worker %s: creating client: %v", nodeID, err)
	}

	w := &testWorker{
		client:   c,
		backends: backends.New(nil),
		nodeID:   nodeID,
		groups:   groups,
	}

	// Register
	info := models.NodeInfo{
		ID:       nodeID,
		Hostname: nodeID,
		Groups:   groups,
		Backends: w.backends.List(),
		Status:   "online",
		LastSeen: time.Now().UTC(),
	}
	if err := c.PutNode(info); err != nil {
		c.Close()
		t.Fatalf("worker %s: registering: %v", nodeID, err)
	}

	// Heartbeat
	hbCtx, hbCancel := context.WithCancel(context.Background())
	w.stopHB = hbCancel
	go func() {
		ticker := time.NewTicker(5 * time.Second) // faster for tests
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				info.LastSeen = time.Now().UTC()
				c.PutNode(info)
			}
		}
	}()

	// Consumer
	stream, err := c.GetStream("commands")
	if err != nil {
		hbCancel()
		c.Close()
		t.Fatalf("worker %s: getting commands stream: %v", nodeID, err)
	}

	filters := []string{"cmd.all.>", fmt.Sprintf("cmd.node.%s.>", nodeID)}
	for _, g := range groups {
		filters = append(filters, fmt.Sprintf("cmd.group.%s.>", g))
	}

	consumer, err := stream.CreateOrUpdateConsumer(context.Background(), jetstream.ConsumerConfig{
		Durable:           fmt.Sprintf("worker-%s", nodeID),
		FilterSubjects:    filters,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		AckPolicy:         jetstream.AckExplicitPolicy,
		InactiveThreshold: 10 * time.Minute,
		MaxAckPending:     1,
	})
	if err != nil {
		hbCancel()
		c.Close()
		t.Fatalf("worker %s: creating consumer: %v", nodeID, err)
	}

	w.context, err = consumer.Consume(w.handleCommand)
	if err != nil {
		hbCancel()
		c.Close()
		t.Fatalf("worker %s: starting consume: %v", nodeID, err)
	}

	t.Cleanup(func() {
		if w.context != nil {
			w.context.Stop()
		}
		hbCancel()
		// Mark offline
		info.Status = "offline"
		c.PutNode(info)
		c.Close()
	})

	return w
}

func (w *testWorker) handleCommand(msg jetstream.Msg) {
	backend := msg.Headers().Get("backend")
	action := msg.Headers().Get("action")
	jobID := msg.Headers().Get("job-id")
	taskIndexStr := msg.Headers().Get("task-index")

	taskIndex := 0
	fmt.Sscanf(taskIndexStr, "%d", &taskIndex)

	var params map[string]string
	if err := json.Unmarshal(msg.Data(), &params); err != nil {
		w.publishResult(jobID, taskIndex, models.ResultFailed, "", fmt.Sprintf("decode params: %s", err))
		msg.Ack()
		return
	}

	b, ok := w.backends.Get(backend)
	if !ok {
		w.publishResult(jobID, taskIndex, models.ResultFailed, "", fmt.Sprintf("unknown backend: %s", backend))
		msg.Ack()
		return
	}

	start := time.Now()
	result, err := b.Run(context.Background(), action, params)
	duration := time.Since(start)

	output := ""
	if result != nil {
		output = result.Output
	}

	if err != nil {
		w.publishResult(jobID, taskIndex, models.ResultFailed, output, err.Error())
	} else {
		_ = duration
		w.publishResult(jobID, taskIndex, models.ResultSuccess, output, "")
	}

	msg.Ack()
}

func (w *testWorker) publishResult(jobID string, taskIndex int, status models.ResultStatus, output, errMsg string) {
	result := models.TaskResult{
		JobID:     jobID,
		TaskIndex: taskIndex,
		NodeID:    w.nodeID,
		Status:    status,
		Output:    output,
		Error:     errMsg,
		Duration:  50 * time.Millisecond,
		Timestamp: time.Now().UTC(),
	}
	w.client.PublishResult(result)
}

// --- Test Harness ---

type testCluster struct {
	env       *testutil.TestEnv
	scheduler *scheduler.Scheduler
	api       *api.API
	apiURL    string
}

func setupCluster(t *testing.T) *testCluster {
	t.Helper()

	env := testutil.NewTestEnv(t)
	reg := registry.New(env.Client)
	sched := scheduler.New(env.Client, reg, env.Config.Scheduler, "")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("starting scheduler: %v", err)
	}

	// Start API on a random port
	env.Config.API.Host = "127.0.0.1"
	env.Config.API.Port = 0 // will be resolved below

	a := api.New(env.Config, env.Client, sched)
	if err := a.Start(); err != nil {
		t.Fatalf("starting API: %v", err)
	}
	t.Cleanup(func() { a.Stop() })

	// The API binds on config port. Since we set 0, we need the actual URL.
	// With port 0, Echo picks a random port — but our Start() uses the config port.
	// Let's use a known random port instead.
	// Actually, Start() uses config port which is 0 — this won't work well.
	// Fall back to using httptest-style testing via ServeHTTP.

	return &testCluster{
		env:       env,
		scheduler: sched,
		api:       a,
	}
}

func (tc *testCluster) postJob(t *testing.T, body string) models.Job {
	t.Helper()
	req, _ := http.NewRequest("POST", "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := tc.doRequest(req)
	if resp.StatusCode != http.StatusAccepted {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /job: status %d, body: %s", resp.StatusCode, b)
	}
	var job models.Job
	json.NewDecoder(resp.Body).Decode(&job)
	return job
}

func (tc *testCluster) getJob(t *testing.T, id string) models.Job {
	t.Helper()
	req, _ := http.NewRequest("GET", "/job/"+id, nil)
	resp := tc.doRequest(req)
	var job models.Job
	json.NewDecoder(resp.Body).Decode(&job)
	return job
}

func (tc *testCluster) cancelJob(t *testing.T, id string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", "/job/"+id+"/cancel", nil)
	resp := tc.doRequest(req)
	return resp.StatusCode
}

func (tc *testCluster) doRequest(req *http.Request) *http.Response {
	rec := &responseRecorder{header: http.Header{}}
	tc.api.ServeHTTP(rec, req)
	return rec.toResponse()
}

// responseRecorder wraps httptest.ResponseRecorder but returns an *http.Response
type responseRecorder struct {
	statusCode int
	header     http.Header
	body       strings.Builder
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}
func (r *responseRecorder) WriteHeader(code int) { r.statusCode = code }
func (r *responseRecorder) toResponse() *http.Response {
	code := r.statusCode
	if code == 0 {
		code = 200
	}
	return &http.Response{
		StatusCode: code,
		Header:     r.header,
		Body:       io.NopCloser(strings.NewReader(r.body.String())),
	}
}

// --- Integration Tests ---

func TestE2E_SubmitAndComplete(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "w1", []string{"web"})
	startTestWorker(t, tc.env, "w2", []string{"web"})
	time.Sleep(200 * time.Millisecond) // let workers register

	job := tc.postJob(t, `{
		"target": {"scope": "all"},
		"tasks": [{"backend": "test", "action": "succeed", "params": {"message": "hello"}}]
	}`)

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCompleted
	}, "job should complete")

	got := tc.getJob(t, job.ID)
	if got.Status != models.JobCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}
	if len(got.Results["0"]) != 2 {
		t.Errorf("step 0 has %d nodes, want 2", len(got.Results["0"]))
	}
}

func TestE2E_MultiStep(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "w1", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	job := tc.postJob(t, `{
		"target": {"scope": "all"},
		"tasks": [
			{"backend": "test", "action": "succeed"},
			{"backend": "ping", "action": "echo", "params": {"message": "step2"}},
			{"backend": "test", "action": "succeed", "params": {"message": "step3"}}
		]
	}`)

	testutil.WaitFor(t, 15*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCompleted
	}, "multi-step job should complete")

	got := tc.getJob(t, job.ID)
	if len(got.Results) != 3 {
		t.Errorf("Results has %d steps, want 3", len(got.Results))
	}
}

func TestE2E_GroupTargeting(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "web-1", []string{"web"})
	startTestWorker(t, tc.env, "web-2", []string{"web"})
	startTestWorker(t, tc.env, "db-1", []string{"db"})
	time.Sleep(200 * time.Millisecond)

	job := tc.postJob(t, `{
		"target": {"scope": "group", "value": "web"},
		"tasks": [{"backend": "test", "action": "succeed"}]
	}`)

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCompleted
	}, "group-targeted job should complete")

	got := tc.getJob(t, job.ID)
	if len(got.Expected) != 2 {
		t.Errorf("Expected len = %d, want 2 (only web nodes)", len(got.Expected))
	}
	if len(got.Results["0"]) != 2 {
		t.Errorf("step 0 has %d nodes, want 2", len(got.Results["0"]))
	}
}

func TestE2E_NodeTargeting(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "target-node", []string{"web"})
	startTestWorker(t, tc.env, "other-node", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	job := tc.postJob(t, `{
		"target": {"scope": "node", "value": "target-node"},
		"tasks": [{"backend": "test", "action": "succeed"}]
	}`)

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCompleted
	}, "node-targeted job should complete")

	got := tc.getJob(t, job.ID)
	if len(got.Expected) != 1 || got.Expected[0] != "target-node" {
		t.Errorf("Expected = %v, want [target-node]", got.Expected)
	}
}

func TestE2E_FailFast(t *testing.T) {
	tc := setupCluster(t)

	// w1 will succeed, w2 will fail (using test/fail backend)
	startTestWorker(t, tc.env, "w1", []string{"web"})
	startTestWorker(t, tc.env, "w2", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	// Submit a job where the action always fails — both nodes will fail
	job := tc.postJob(t, `{
		"target": {"scope": "all"},
		"strategy": "fail-fast",
		"tasks": [
			{"backend": "test", "action": "fail", "params": {"message": "boom"}},
			{"backend": "test", "action": "succeed"}
		]
	}`)

	testutil.WaitFor(t, 10*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobFailed
	}, "job should fail")

	got := tc.getJob(t, job.ID)
	if got.Status != models.JobFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	// Step 1 should have skipped results (fail-fast now records skips)
	for nodeID, nr := range got.Results["1"] {
		if nr.Status != models.ResultSkipped {
			t.Errorf("step 1 node %s status = %q, want skipped", nodeID, nr.Status)
		}
	}
}

func TestE2E_Cancel(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "w1", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	// Submit a slow job
	job := tc.postJob(t, `{
		"target": {"scope": "all"},
		"tasks": [{"backend": "test", "action": "sleep", "params": {"duration": "30s"}}]
	}`)

	// Wait for it to start running
	testutil.WaitFor(t, 5*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobRunning
	}, "job should start running")

	// Cancel
	code := tc.cancelJob(t, job.ID)
	if code != http.StatusOK {
		t.Errorf("cancel status = %d, want %d", code, http.StatusOK)
	}

	testutil.WaitFor(t, 5*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCancelled
	}, "job should become cancelled")
}

func TestE2E_NodeList(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "n1", []string{"web"})
	startTestWorker(t, tc.env, "n2", []string{"db"})
	time.Sleep(200 * time.Millisecond)

	req, _ := http.NewRequest("GET", "/nodes", nil)
	resp := tc.doRequest(req)

	var nodes []models.NodeInfo
	json.NewDecoder(resp.Body).Decode(&nodes)

	if len(nodes) != 2 {
		t.Errorf("nodes len = %d, want 2", len(nodes))
	}
}

func TestE2E_JobList(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "w1", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	// Submit 3 jobs
	for i := 0; i < 3; i++ {
		tc.postJob(t, `{
			"target": {"scope": "all"},
			"tasks": [{"backend": "test", "action": "succeed"}]
		}`)
	}

	// Wait for all to complete
	time.Sleep(2 * time.Second)

	req, _ := http.NewRequest("GET", "/jobs", nil)
	resp := tc.doRequest(req)

	var jobs []models.Job
	json.NewDecoder(resp.Body).Decode(&jobs)

	if len(jobs) != 3 {
		t.Errorf("jobs len = %d, want 3", len(jobs))
	}
}

func TestE2E_MultiController(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Two schedulers with different controller IDs sharing the same NATS
	reg := registry.New(env.Client)
	sched1 := scheduler.New(env.Client, reg, env.Config.Scheduler, "ctrl-1")
	sched2 := scheduler.New(env.Client, reg, env.Config.Scheduler, "ctrl-2")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := sched1.Start(ctx); err != nil {
		t.Fatalf("starting sched1: %v", err)
	}
	if err := sched2.Start(ctx); err != nil {
		t.Fatalf("starting sched2: %v", err)
	}

	a := api.New(env.Config, env.Client, sched1)
	tc := &testCluster{env: env, scheduler: sched1, api: a}

	startTestWorker(t, env, "w1", []string{"web"})
	startTestWorker(t, env, "w2", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	// Submit multiple jobs — they'll be distributed across both schedulers
	totalJobs := 6
	for i := 0; i < totalJobs; i++ {
		tc.postJob(t, fmt.Sprintf(`{
			"target": {"scope": "all"},
			"tasks": [{"backend": "test", "action": "succeed", "params": {"message": "job-%d"}}]
		}`, i))
	}

	// All jobs should complete
	jobs, err := env.Client.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}

	for _, job := range jobs {
		testutil.WaitFor(t, 15*time.Second, func() bool {
			j, _, _ := env.Client.GetJob(job.ID)
			return j.Status == models.JobCompleted
		}, fmt.Sprintf("job %s should complete", job.ID))
	}

	// Verify all completed with an owner set
	jobs, _ = env.Client.ListJobs()
	for _, job := range jobs {
		if job.Status != models.JobCompleted {
			t.Errorf("job %s status = %q, want completed", job.ID, job.Status)
		}
		if job.Owner == "" {
			t.Errorf("job %s has no owner", job.ID)
		}
		if job.Owner != "ctrl-1" && job.Owner != "ctrl-2" {
			t.Errorf("job %s owner = %q, want ctrl-1 or ctrl-2", job.ID, job.Owner)
		}
	}
}

func TestE2E_Pipeline(t *testing.T) {
	tc := setupCluster(t)

	startTestWorker(t, tc.env, "w1", []string{"web"})
	startTestWorker(t, tc.env, "w2", []string{"web"})
	time.Sleep(200 * time.Millisecond)

	// Submit a mixed job: barrier → pipeline → barrier
	job := tc.postJob(t, `{
		"target": {"scope": "all"},
		"tasks": [
			{"backend": "test", "action": "succeed", "params": {"message": "barrier-1"}},
			{"tasks": [
				{"backend": "test", "action": "succeed", "params": {"message": "pipe-step-1"}},
				{"backend": "ping", "action": "echo", "params": {"message": "pipe-step-2"}}
			]},
			{"backend": "test", "action": "succeed", "params": {"message": "barrier-2"}}
		]
	}`)

	testutil.WaitFor(t, 15*time.Second, func() bool {
		j := tc.getJob(t, job.ID)
		return j.Status == models.JobCompleted
	}, "pipeline E2E job should complete")

	got := tc.getJob(t, job.ID)
	if got.Status != models.JobCompleted {
		t.Errorf("Status = %q, want completed", got.Status)
	}

	// 4 leaf steps: barrier(0) + pipeline(1,2) + barrier(3)
	if len(got.Results) != 4 {
		t.Errorf("Results has %d steps, want 4", len(got.Results))
	}

	// Each step should have 2 node results
	for step := 0; step < 4; step++ {
		key := fmt.Sprintf("%d", step)
		if len(got.Results[key]) != 2 {
			t.Errorf("step %d has %d nodes, want 2", step, len(got.Results[key]))
		}
	}
}
