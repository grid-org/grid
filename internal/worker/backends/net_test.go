package backends

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNetBackend_Actions(t *testing.T) {
	b := &NetBackend{}
	want := map[string]bool{"ping": true, "resolve": true, "port": true, "interfaces": true, "traceroute": true}
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

func TestNetBackend_Resolve(t *testing.T) {
	b := &NetBackend{}
	result, err := b.Run(context.Background(), "resolve", map[string]string{"host": "localhost"})
	if err != nil {
		t.Fatalf("resolve error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["host"] != "localhost" {
		t.Errorf("expected host=localhost, got %v", data["host"])
	}
}

func TestNetBackend_Port(t *testing.T) {
	b := &NetBackend{}
	// Test against a port that's unlikely to be open
	result, err := b.Run(context.Background(), "port", map[string]string{
		"host":    "127.0.0.1",
		"port":    "1",
		"timeout": "1s",
	})
	if err != nil {
		t.Fatalf("port error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Port 1 should be closed on localhost
	if _, ok := data["open"]; !ok {
		t.Error("missing 'open' field in output")
	}
}

func TestNetBackend_Interfaces(t *testing.T) {
	b := &NetBackend{}
	result, err := b.Run(context.Background(), "interfaces", nil)
	if err != nil {
		t.Fatalf("interfaces error: %v", err)
	}

	var data []map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected at least one interface")
	}

	// Check first interface has expected fields
	iface := data[0]
	for _, key := range []string{"name", "addresses", "flags"} {
		if _, ok := iface[key]; !ok {
			t.Errorf("missing key %q in interface", key)
		}
	}
}

func TestNetBackend_MissingHost(t *testing.T) {
	b := &NetBackend{}
	_, err := b.Run(context.Background(), "resolve", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestNetBackend_InvalidPort(t *testing.T) {
	b := &NetBackend{}
	_, err := b.Run(context.Background(), "port", map[string]string{
		"host": "localhost",
		"port": "99999",
	})
	if err == nil {
		t.Fatal("expected error for invalid port")
	}
}

func TestNetBackend_UnknownAction(t *testing.T) {
	b := &NetBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
