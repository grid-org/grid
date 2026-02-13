package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/scheduler"
	"github.com/grid-org/grid/internal/testutil"
)

func setupAPI(t *testing.T) (*testutil.TestEnv, *api.API, *scheduler.Scheduler) {
	t.Helper()
	env := testutil.NewTestEnv(t)

	// Register a node so target resolution works
	env.RegisterNodes(t, testutil.OnlineNode("test-node", "web"))

	reg := registry.New(env.Client)
	sched := scheduler.New(env.Client, reg, env.Config.Scheduler)
	a := api.New(env.Config, env.Client, sched)

	return env, a, sched
}

func TestPostJob_Valid(t *testing.T) {
	_, a, _ := setupAPI(t)

	body := `{"target":{"scope":"all"},"tasks":[{"backend":"test","action":"succeed"}]}`
	req := httptest.NewRequest("POST", "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}

	var job models.Job
	if err := json.NewDecoder(rec.Body).Decode(&job); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if job.ID == "" {
		t.Error("job ID should not be empty")
	}
	if job.Status != models.JobPending {
		t.Errorf("Status = %q, want pending", job.Status)
	}
}

func TestPostJob_NoTasks(t *testing.T) {
	_, a, _ := setupAPI(t)

	body := `{"target":{"scope":"all"},"tasks":[]}`
	req := httptest.NewRequest("POST", "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPostJob_EmptyBody(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("POST", "/job", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	// Empty tasks → 400
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetJob_Found(t *testing.T) {
	env, a, _ := setupAPI(t)

	// Pre-create a job
	job := models.Job{
		ID:     "get-test",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Phase{{Backend: "test", Action: "succeed"}},
		Status: models.JobRunning,
	}
	env.Client.CreateJob(job)

	req := httptest.NewRequest("GET", "/job/get-test", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got models.Job
	json.NewDecoder(rec.Body).Decode(&got)
	if got.ID != "get-test" {
		t.Errorf("ID = %q, want get-test", got.ID)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("GET", "/job/nonexistent", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestListJobs(t *testing.T) {
	env, a, _ := setupAPI(t)

	for _, id := range []string{"j1", "j2"} {
		env.Client.CreateJob(models.Job{
			ID:     id,
			Target: models.Target{Scope: "all"},
			Tasks:  []models.Phase{{Backend: "test", Action: "succeed"}},
		})
	}

	req := httptest.NewRequest("GET", "/jobs", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var jobs []models.Job
	json.NewDecoder(rec.Body).Decode(&jobs)
	if len(jobs) != 2 {
		t.Errorf("len = %d, want 2", len(jobs))
	}
}

func TestCancelJob_Pending(t *testing.T) {
	env, a, sched := setupAPI(t)
	_ = sched // scheduler not started, so job stays pending

	// Create a pending job via Enqueue (not Start)
	job := models.Job{
		ID:     "cancel-api",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Phase{{Backend: "test", Action: "succeed"}},
	}
	sched.Enqueue(job)

	req := httptest.NewRequest("POST", "/job/cancel-api/cancel", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	got, _, _ := env.Client.GetJob("cancel-api")
	if got.Status != models.JobCancelled {
		t.Errorf("Status = %q, want cancelled", got.Status)
	}
}

func TestListNodes(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("GET", "/nodes", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var nodes []models.NodeInfo
	json.NewDecoder(rec.Body).Decode(&nodes)
	if len(nodes) != 1 { // setup registered one node
		t.Errorf("len = %d, want 1", len(nodes))
	}
}

func TestGetNode_Found(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("GET", "/node/test-node", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var node models.NodeInfo
	json.NewDecoder(rec.Body).Decode(&node)
	if node.ID != "test-node" {
		t.Errorf("ID = %q, want test-node", node.ID)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("GET", "/node/nonexistent", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestGetStatus(t *testing.T) {
	_, a, _ := setupAPI(t)

	req := httptest.NewRequest("GET", "/status", nil)
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "active" {
		t.Errorf("status = %q, want active", resp["status"])
	}
}

func TestPostJob_429_WhenQueueFull(t *testing.T) {
	env := testutil.NewTestEnv(t)
	env.RegisterNodes(t, testutil.OnlineNode("test-node", "web"))

	// Tiny pending limit to trigger queue full quickly
	env.Config.Scheduler.MaxPending = 2
	env.Config.Scheduler.MaxConcurrent = 1

	reg := registry.New(env.Client)
	sched := scheduler.New(env.Client, reg, env.Config.Scheduler)
	a := api.New(env.Config, env.Client, sched)

	body := `{"target":{"scope":"all"},"tasks":[{"backend":"test","action":"succeed"}]}`

	// Fill the pending queue
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("POST", "/job", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		a.ServeHTTP(rec, req)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("fill request %d: status = %d, want %d", i, rec.Code, http.StatusAccepted)
		}
	}

	// Next request should get 429
	req := httptest.NewRequest("POST", "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d; body: %s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
}

func TestPostJob_WithStrategy(t *testing.T) {
	_, a, _ := setupAPI(t)

	body := `{"target":{"scope":"all"},"tasks":[{"backend":"test","action":"succeed"}],"strategy":"continue","timeout":"30m"}`
	req := httptest.NewRequest("POST", "/job", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	var job models.Job
	json.NewDecoder(rec.Body).Decode(&job)
	if job.Strategy != models.StrategyContinue {
		t.Errorf("Strategy = %q, want continue", job.Strategy)
	}
	if job.Timeout != "30m" {
		t.Errorf("Timeout = %q, want 30m", job.Timeout)
	}
}

