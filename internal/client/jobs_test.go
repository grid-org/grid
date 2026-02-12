package client_test

import (
	"testing"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/testutil"
)

func TestBuildCommandSubject(t *testing.T) {
	tests := []struct {
		name   string
		target models.Target
		task   models.Task
		want   string
	}{
		{
			"all scope",
			models.Target{Scope: "all"},
			models.Task{Backend: "apt", Action: "install"},
			"cmd.all.apt.install",
		},
		{
			"group scope",
			models.Target{Scope: "group", Value: "web"},
			models.Task{Backend: "systemd", Action: "restart"},
			"cmd.group.web.systemd.restart",
		},
		{
			"node scope",
			models.Target{Scope: "node", Value: "web-01"},
			models.Task{Backend: "rke2", Action: "start"},
			"cmd.node.web-01.rke2.start",
		},
		{
			"empty scope defaults to all",
			models.Target{Scope: ""},
			models.Task{Backend: "ping", Action: "echo"},
			"cmd.all.ping.echo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.BuildCommandSubject(tt.target, tt.task)
			if got != tt.want {
				t.Errorf("BuildCommandSubject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCreateJob(t *testing.T) {
	env := testutil.NewTestEnv(t)

	job := models.Job{
		ID:     "test-create-1",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
		Status: models.JobPending,
	}

	created, err := env.Client.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if created.CreatedAt.IsZero() {
		t.Error("CreatedAt should be set")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be set")
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Error("CreatedAt and UpdatedAt should be equal on creation")
	}
}

func TestGetJob(t *testing.T) {
	env := testutil.NewTestEnv(t)

	job := models.Job{
		ID:       "test-get-1",
		Target:   models.Target{Scope: "group", Value: "web"},
		Tasks:    []models.Task{{Backend: "apt", Action: "install", Params: map[string]string{"package": "curl"}}},
		Status:   models.JobPending,
		Expected: []string{"web-01"},
	}

	_, err := env.Client.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, err := env.Client.GetJob("test-get-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if got.ID != "test-get-1" {
		t.Errorf("ID = %q, want test-get-1", got.ID)
	}
	if got.Target.Value != "web" {
		t.Errorf("Target.Value = %q, want web", got.Target.Value)
	}
	if len(got.Expected) != 1 || got.Expected[0] != "web-01" {
		t.Errorf("Expected = %v, want [web-01]", got.Expected)
	}
}

func TestGetJob_NotFound(t *testing.T) {
	env := testutil.NewTestEnv(t)

	_, err := env.Client.GetJob("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent job")
	}
}

func TestUpdateJob(t *testing.T) {
	env := testutil.NewTestEnv(t)

	job := models.Job{
		ID:     "test-update-1",
		Target: models.Target{Scope: "all"},
		Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
		Status: models.JobPending,
	}

	created, err := env.Client.CreateJob(job)
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	created.Status = models.JobRunning
	created.Step = 1
	if err := env.Client.UpdateJob(created); err != nil {
		t.Fatalf("UpdateJob: %v", err)
	}

	got, err := env.Client.GetJob("test-update-1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	if got.Status != models.JobRunning {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Step != 1 {
		t.Errorf("Step = %d, want 1", got.Step)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt")
	}
}

func TestListJobs(t *testing.T) {
	env := testutil.NewTestEnv(t)

	for _, id := range []string{"list-1", "list-2", "list-3"} {
		job := models.Job{
			ID:     id,
			Target: models.Target{Scope: "all"},
			Tasks:  []models.Task{{Backend: "test", Action: "succeed"}},
		}
		if _, err := env.Client.CreateJob(job); err != nil {
			t.Fatalf("CreateJob(%s): %v", id, err)
		}
	}

	jobs, err := env.Client.ListJobs()
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}

	if len(jobs) != 3 {
		t.Errorf("len = %d, want 3", len(jobs))
	}
}
