package backends

import (
	"context"
	"encoding/json"
	"testing"
)

func TestSysinfoBackend_Actions(t *testing.T) {
	b := &SysinfoBackend{}
	want := map[string]bool{"os": true, "cpu": true, "memory": true, "disk": true, "load": true, "uptime": true}
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

func TestSysinfoBackend_OS(t *testing.T) {
	b := &SysinfoBackend{}
	result, err := b.Run(context.Background(), "os", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}

	for _, key := range []string{"hostname", "os", "arch", "kernel", "uptime_seconds", "boot_time"} {
		if _, ok := data[key]; !ok {
			t.Errorf("missing key %q in output", key)
		}
	}
}

func TestSysinfoBackend_CPU(t *testing.T) {
	b := &SysinfoBackend{}
	result, err := b.Run(context.Background(), "cpu", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if count, ok := data["count"].(float64); !ok || count < 1 {
		t.Errorf("expected count >= 1, got %v", data["count"])
	}
}

func TestSysinfoBackend_Memory(t *testing.T) {
	b := &SysinfoBackend{}
	result, err := b.Run(context.Background(), "memory", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if total, ok := data["total"].(float64); !ok || total <= 0 {
		t.Errorf("expected total > 0, got %v", data["total"])
	}
}

func TestSysinfoBackend_Disk(t *testing.T) {
	b := &SysinfoBackend{}

	// Default mount
	result, err := b.Run(context.Background(), "disk", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if total, ok := data["total"].(float64); !ok || total <= 0 {
		t.Errorf("expected total > 0, got %v", data["total"])
	}
}

func TestSysinfoBackend_Load(t *testing.T) {
	b := &SysinfoBackend{}
	result, err := b.Run(context.Background(), "load", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	for _, key := range []string{"load1", "load5", "load15"} {
		if _, ok := data[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}
}

func TestSysinfoBackend_Uptime(t *testing.T) {
	b := &SysinfoBackend{}
	result, err := b.Run(context.Background(), "uptime", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if uptime, ok := data["uptime_seconds"].(float64); !ok || uptime <= 0 {
		t.Errorf("expected uptime > 0, got %v", data["uptime_seconds"])
	}
}

func TestSysinfoBackend_UnknownAction(t *testing.T) {
	b := &SysinfoBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
