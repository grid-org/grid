package backends

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthBackend_Actions(t *testing.T) {
	b := &HealthBackend{}
	want := map[string]bool{"http": true, "tcp": true, "disk_space": true, "memory": true, "load": true, "process": true}
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

func TestHealthBackend_HTTP(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	}))
	defer ts.Close()

	b := &HealthBackend{}
	result, err := b.Run(context.Background(), "http", map[string]string{
		"url":             ts.URL,
		"expected_status": "200",
	})
	if err != nil {
		t.Fatalf("http check error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !data["matches"].(bool) {
		t.Error("expected matches=true")
	}
	if data["status_code"].(float64) != 200 {
		t.Errorf("expected status 200, got %v", data["status_code"])
	}
}

func TestHealthBackend_HTTPBadScheme(t *testing.T) {
	b := &HealthBackend{}
	_, err := b.Run(context.Background(), "http", map[string]string{
		"url": "ftp://example.com",
	})
	if err == nil {
		t.Fatal("expected error for ftp scheme")
	}
}

func TestHealthBackend_HTTPBadMethod(t *testing.T) {
	b := &HealthBackend{}
	_, err := b.Run(context.Background(), "http", map[string]string{
		"url":    "http://localhost",
		"method": "POST",
	})
	if err == nil {
		t.Fatal("expected error for POST method")
	}
}

func TestHealthBackend_TCP(t *testing.T) {
	b := &HealthBackend{}
	result, err := b.Run(context.Background(), "tcp", map[string]string{
		"host":    "127.0.0.1",
		"port":    "1",
		"timeout": "1s",
	})
	if err != nil {
		t.Fatalf("tcp check error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Port 1 unlikely to be open
	if _, ok := data["reachable"]; !ok {
		t.Error("missing 'reachable' field")
	}
}

func TestHealthBackend_DiskSpace(t *testing.T) {
	b := &HealthBackend{}
	result, err := b.Run(context.Background(), "disk_space", map[string]string{
		"mount":             "/",
		"threshold_percent": "99",
	})
	if err != nil {
		t.Fatalf("disk_space error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := data["healthy"]; !ok {
		t.Error("missing 'healthy' field")
	}
}

func TestHealthBackend_Memory(t *testing.T) {
	b := &HealthBackend{}
	result, err := b.Run(context.Background(), "memory", nil)
	if err != nil {
		t.Fatalf("memory error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := data["usage_percent"]; !ok {
		t.Error("missing 'usage_percent' field")
	}
}

func TestHealthBackend_Load(t *testing.T) {
	b := &HealthBackend{}
	result, err := b.Run(context.Background(), "load", nil)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := data["load1"]; !ok {
		t.Error("missing 'load1' field")
	}
}

func TestHealthBackend_UnknownAction(t *testing.T) {
	b := &HealthBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
