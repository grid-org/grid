package backends

import (
	"context"
	"encoding/json"
	"testing"
)

func TestProcBackend_Actions(t *testing.T) {
	b := &ProcBackend{}
	want := map[string]bool{"list": true, "top": true, "find": true, "count": true}
	for _, a := range b.Actions() {
		if !want[a] {
			t.Errorf("unexpected action %q", a)
		}
		delete(want, a)
	}
	for a := range want {
		t.Errorf("missing action %q", a)
	}
}

func TestProcBackend_List(t *testing.T) {
	b := &ProcBackend{}
	result, err := b.Run(context.Background(), "list", nil)
	if err != nil {
		t.Fatalf("list error: %v", err)
	}

	var data []procInfo
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected at least one process")
	}
}

func TestProcBackend_Top(t *testing.T) {
	b := &ProcBackend{}
	result, err := b.Run(context.Background(), "top", map[string]string{"count": "5"})
	if err != nil {
		t.Fatalf("top error: %v", err)
	}

	var data []procInfo
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(data) > 5 {
		t.Errorf("expected max 5, got %d", len(data))
	}
}

func TestProcBackend_Count(t *testing.T) {
	b := &ProcBackend{}
	result, err := b.Run(context.Background(), "count", nil)
	if err != nil {
		t.Fatalf("count error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if total, ok := data["total"].(float64); !ok || total < 1 {
		t.Errorf("expected total > 0, got %v", data["total"])
	}
}

func TestProcBackend_Find(t *testing.T) {
	b := &ProcBackend{}
	_, err := b.Run(context.Background(), "find", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing name param")
	}
}

func TestProcBackend_UnknownAction(t *testing.T) {
	b := &ProcBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
