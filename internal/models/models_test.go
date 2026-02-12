package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
)

func TestJob_JSONRoundtrip(t *testing.T) {
	original := Job{
		ID:       "job-1",
		Target:   Target{Scope: "group", Value: "web"},
		Tasks:    []Task{{Backend: "apt", Action: "install", Params: map[string]string{"package": "curl"}}},
		Strategy: StrategyFailFast,
		Timeout:  "30m",
		Status:   JobRunning,
		Step:     1,
		Expected: []string{"web-01", "web-02"},
		Results: JobResults{
			"0": {
				"web-01": NodeResult{Status: ResultSuccess, Output: "ok", Duration: "2.3s"},
				"web-02": NodeResult{Status: ResultSuccess, Output: "ok", Duration: "1.8s", Attempts: 2},
			},
		},
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Job
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if decoded.Target.Scope != original.Target.Scope || decoded.Target.Value != original.Target.Value {
		t.Errorf("Target = %+v, want %+v", decoded.Target, original.Target)
	}
	if decoded.Strategy != original.Strategy {
		t.Errorf("Strategy = %q, want %q", decoded.Strategy, original.Strategy)
	}
	if decoded.Timeout != original.Timeout {
		t.Errorf("Timeout = %q, want %q", decoded.Timeout, original.Timeout)
	}
	if decoded.Status != original.Status {
		t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
	}
	if decoded.Step != original.Step {
		t.Errorf("Step = %d, want %d", decoded.Step, original.Step)
	}
	if len(decoded.Expected) != len(original.Expected) {
		t.Errorf("Expected len = %d, want %d", len(decoded.Expected), len(original.Expected))
	}
	if len(decoded.Results) != 1 {
		t.Fatalf("Results has %d steps, want 1", len(decoded.Results))
	}
	nr := decoded.Results["0"]["web-02"]
	if nr.Attempts != 2 {
		t.Errorf("Attempts = %d, want 2", nr.Attempts)
	}
}

func TestJob_OmitEmpty(t *testing.T) {
	job := Job{
		ID:     "job-2",
		Target: Target{Scope: "all"},
		Tasks:  []Task{{Backend: "ping", Action: "echo"}},
		Status: JobPending,
	}

	data, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := string(data)
	// Timeout and results should be omitted when empty
	if containsKey(raw, "timeout") {
		t.Error("expected timeout to be omitted")
	}
	if containsKey(raw, "results") {
		t.Error("expected results to be omitted")
	}
}

func TestTaskResult_JSONRoundtrip(t *testing.T) {
	original := TaskResult{
		JobID:     "job-1",
		TaskIndex: 2,
		NodeID:    "web-01",
		Status:    ResultFailed,
		Output:    "some output",
		Error:     "something broke",
		Duration:  2*time.Second + 300*time.Millisecond,
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded TaskResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.JobID != original.JobID {
		t.Errorf("JobID = %q, want %q", decoded.JobID, original.JobID)
	}
	if decoded.Status != ResultFailed {
		t.Errorf("Status = %q, want %q", decoded.Status, ResultFailed)
	}
	if decoded.Error != original.Error {
		t.Errorf("Error = %q, want %q", decoded.Error, original.Error)
	}
}

func TestNodeInfo_JSONRoundtrip(t *testing.T) {
	original := NodeInfo{
		ID:       "node-1",
		Hostname: "server1.local",
		Groups:   []string{"web", "prod"},
		Backends: []string{"apt", "systemd"},
		Status:   "online",
		LastSeen: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded NodeInfo
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != original.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, original.ID)
	}
	if len(decoded.Groups) != 2 {
		t.Errorf("Groups len = %d, want 2", len(decoded.Groups))
	}
	if decoded.Status != "online" {
		t.Errorf("Status = %q, want online", decoded.Status)
	}
}

func TestJobFile_YAMLRoundtrip(t *testing.T) {
	original := JobFile{
		Target:   Target{Scope: "group", Value: "web"},
		Tasks:    []Task{{Backend: "apt", Action: "update"}, {Backend: "apt", Action: "install", Params: map[string]string{"package": "nginx"}}},
		Strategy: StrategyContinue,
		Timeout:  "1h",
	}

	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("yaml marshal: %v", err)
	}

	var decoded JobFile
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml unmarshal: %v", err)
	}

	if decoded.Target.Scope != "group" || decoded.Target.Value != "web" {
		t.Errorf("Target = %+v, want group:web", decoded.Target)
	}
	if len(decoded.Tasks) != 2 {
		t.Errorf("Tasks len = %d, want 2", len(decoded.Tasks))
	}
	if decoded.Strategy != StrategyContinue {
		t.Errorf("Strategy = %q, want continue", decoded.Strategy)
	}
}

func containsKey(jsonStr, key string) bool {
	var m map[string]json.RawMessage
	json.Unmarshal([]byte(jsonStr), &m)
	_, ok := m[key]
	return ok
}
