package backends

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPingBackend_Echo(t *testing.T) {
	p := &PingBackend{}

	tests := []struct {
		name    string
		params  map[string]string
		want    string
	}{
		{"default", map[string]string{}, "pong"},
		{"custom message", map[string]string{"message": "hello"}, "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Run(context.Background(), "echo", tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Output != tt.want {
				t.Errorf("output = %q, want %q", result.Output, tt.want)
			}
		})
	}
}

func TestPingBackend_Hostname(t *testing.T) {
	p := &PingBackend{}

	result, err := p.Run(context.Background(), "hostname", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected, _ := os.Hostname()
	if result.Output != expected {
		t.Errorf("output = %q, want %q", result.Output, expected)
	}
}

func TestPingBackend_Sleep(t *testing.T) {
	p := &PingBackend{}

	start := time.Now()
	result, err := p.Run(context.Background(), "sleep", map[string]string{"duration": "100ms"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "slept 100ms" {
		t.Errorf("output = %q, want %q", result.Output, "slept 100ms")
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 100ms", elapsed)
	}
}

func TestPingBackend_SleepCancellation(t *testing.T) {
	p := &PingBackend{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.Run(ctx, "sleep", map[string]string{"duration": "5s"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if elapsed > 1*time.Second {
		t.Errorf("cancellation took too long: %v", elapsed)
	}
}

func TestPingBackend_UnknownAction(t *testing.T) {
	p := &PingBackend{}
	_, err := p.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestPingBackend_Actions(t *testing.T) {
	p := &PingBackend{}
	actions := p.Actions()

	want := map[string]bool{"echo": true, "hostname": true, "sleep": true}
	for _, a := range actions {
		if !want[a] {
			t.Errorf("unexpected action %q", a)
		}
		delete(want, a)
	}
	for a := range want {
		t.Errorf("missing action %q", a)
	}
}
