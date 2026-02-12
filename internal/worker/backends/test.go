package backends

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

// TestBackend provides deterministic and configurable actions for integration
// testing. No shell execution -- pure Go.
type TestBackend struct{}

func init() {
	registerBackend("test", &TestBackend{})
}

func (t *TestBackend) Actions() []string {
	return []string{"succeed", "fail", "sleep", "flaky", "output"}
}

func (t *TestBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "succeed":
		if err := optionalDelay(ctx, params["delay"]); err != nil {
			return nil, err
		}
		msg := params["message"]
		if msg == "" {
			msg = "ok"
		}
		return &Result{Output: msg}, nil

	case "fail":
		if err := optionalDelay(ctx, params["delay"]); err != nil {
			return nil, err
		}
		msg := params["message"]
		if msg == "" {
			msg = "intentional failure"
		}
		return nil, fmt.Errorf("%s", msg)

	case "sleep":
		dur := params["duration"]
		if dur == "" {
			dur = "1s"
		}
		d, err := time.ParseDuration(dur)
		if err != nil {
			return nil, fmt.Errorf("test sleep: invalid duration %q: %w", dur, err)
		}
		select {
		case <-time.After(d):
			return &Result{Output: fmt.Sprintf("slept %s", d)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}

	case "flaky":
		if err := optionalDelay(ctx, params["delay"]); err != nil {
			return nil, err
		}
		rate := 50
		if r := params["rate"]; r != "" {
			if _, err := fmt.Sscanf(r, "%d", &rate); err != nil || rate < 0 || rate > 100 {
				return nil, fmt.Errorf("test flaky: rate must be 0-100, got %q", r)
			}
		}
		if rand.Intn(100) < rate {
			return nil, fmt.Errorf("flaky failure (rate=%d%%)", rate)
		}
		return &Result{Output: fmt.Sprintf("flaky success (rate=%d%%)", rate)}, nil

	case "output":
		lines := 1
		width := 80
		if l := params["lines"]; l != "" {
			fmt.Sscanf(l, "%d", &lines)
		}
		if w := params["width"]; w != "" {
			fmt.Sscanf(w, "%d", &width)
		}
		if lines < 1 {
			lines = 1
		}
		if lines > 10000 {
			lines = 10000
		}
		if width < 1 {
			width = 1
		}
		if width > 1000 {
			width = 1000
		}
		var sb strings.Builder
		line := strings.Repeat("x", width)
		for i := 0; i < lines; i++ {
			if i > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(line)
		}
		return &Result{Output: sb.String()}, nil

	default:
		return nil, fmt.Errorf("test: unknown action %q", action)
	}
}

func optionalDelay(ctx context.Context, delay string) error {
	if delay == "" {
		return nil
	}
	d, err := time.ParseDuration(delay)
	if err != nil {
		return fmt.Errorf("invalid delay %q: %w", delay, err)
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
