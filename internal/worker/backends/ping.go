package backends

import (
	"context"
	"fmt"
	"os"
	"time"
)

// PingBackend is a lightweight backend for health checks and testing.
// All actions are pure Go -- no shell execution.
type PingBackend struct{}

func init() {
	registerBackend("ping", &PingBackend{})
}

func (p *PingBackend) Actions() []string {
	return []string{"echo", "hostname", "sleep"}
}

func (p *PingBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "echo":
		msg := params["message"]
		if msg == "" {
			msg = "pong"
		}
		return &Result{Output: msg}, nil

	case "hostname":
		name, err := os.Hostname()
		if err != nil {
			return nil, fmt.Errorf("ping hostname: %w", err)
		}
		return &Result{Output: name}, nil

	case "sleep":
		dur := params["duration"]
		if dur == "" {
			dur = "1s"
		}
		d, err := time.ParseDuration(dur)
		if err != nil {
			return nil, fmt.Errorf("ping sleep: invalid duration %q: %w", dur, err)
		}
		select {
		case <-time.After(d):
			return &Result{Output: fmt.Sprintf("slept %s", d)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}

	default:
		return nil, fmt.Errorf("ping: unknown action %q", action)
	}
}
