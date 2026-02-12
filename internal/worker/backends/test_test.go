package backends

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestTestBackend_Succeed(t *testing.T) {
	b := &TestBackend{}

	tests := []struct {
		name   string
		params map[string]string
		want   string
	}{
		{"default message", map[string]string{}, "ok"},
		{"custom message", map[string]string{"message": "done"}, "done"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := b.Run(context.Background(), "succeed", tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Output != tt.want {
				t.Errorf("output = %q, want %q", result.Output, tt.want)
			}
		})
	}
}

func TestTestBackend_Fail(t *testing.T) {
	b := &TestBackend{}

	tests := []struct {
		name      string
		params    map[string]string
		wantError string
	}{
		{"default", map[string]string{}, "intentional failure"},
		{"custom", map[string]string{"message": "boom"}, "boom"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := b.Run(context.Background(), "fail", tt.params)
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Error() != tt.wantError {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantError)
			}
		})
	}
}

func TestTestBackend_Sleep(t *testing.T) {
	b := &TestBackend{}

	start := time.Now()
	result, err := b.Run(context.Background(), "sleep", map[string]string{"duration": "100ms"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Output != "slept 100ms" {
		t.Errorf("output = %q", result.Output)
	}
	if elapsed < 100*time.Millisecond {
		t.Errorf("elapsed = %v, want >= 100ms", elapsed)
	}
}

func TestTestBackend_SleepCancellation(t *testing.T) {
	b := &TestBackend{}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := b.Run(ctx, "sleep", map[string]string{"duration": "5s"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestTestBackend_Output(t *testing.T) {
	b := &TestBackend{}

	tests := []struct {
		name      string
		params    map[string]string
		wantLines int
		wantWidth int
	}{
		{"defaults", map[string]string{}, 1, 80},
		{"custom", map[string]string{"lines": "5", "width": "10"}, 5, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := b.Run(context.Background(), "output", tt.params)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			lines := strings.Split(result.Output, "\n")
			if len(lines) != tt.wantLines {
				t.Errorf("lines = %d, want %d", len(lines), tt.wantLines)
			}
			if len(lines[0]) != tt.wantWidth {
				t.Errorf("width = %d, want %d", len(lines[0]), tt.wantWidth)
			}
		})
	}
}

func TestTestBackend_Flaky(t *testing.T) {
	b := &TestBackend{}

	// With rate=0, should always succeed
	for i := 0; i < 20; i++ {
		_, err := b.Run(context.Background(), "flaky", map[string]string{"rate": "0"})
		if err != nil {
			t.Fatalf("flaky with rate=0 should not fail: %v", err)
		}
	}

	// With rate=100, should always fail
	for i := 0; i < 20; i++ {
		_, err := b.Run(context.Background(), "flaky", map[string]string{"rate": "100"})
		if err == nil {
			t.Fatal("flaky with rate=100 should always fail")
		}
	}
}

func TestTestBackend_UnknownAction(t *testing.T) {
	b := &TestBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}

func TestTestBackend_Actions(t *testing.T) {
	b := &TestBackend{}
	actions := b.Actions()

	want := map[string]bool{"succeed": true, "fail": true, "sleep": true, "flaky": true, "output": true}
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
