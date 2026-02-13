package backends

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// unitNameRe validates systemd unit names: alphanumeric plus -._@
var unitNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._@-]*$`)

type JournaldBackend struct{}

func init() {
	registerBackend("journald", &JournaldBackend{})
}

func (j *JournaldBackend) Actions() []string {
	return []string{"logs", "tail", "boots", "disk_usage"}
}

func (j *JournaldBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "logs":
		return j.logs(ctx, params)
	case "tail":
		return j.tail(ctx, params)
	case "boots":
		return j.boots(ctx)
	case "disk_usage":
		return j.diskUsage(ctx)
	default:
		return nil, fmt.Errorf("journald: unknown action %q", action)
	}
}

func validateUnit(unit string) error {
	if !unitNameRe.MatchString(unit) {
		return fmt.Errorf("journald: invalid unit name %q (alphanumeric, -, ., _, @ only)", unit)
	}
	return nil
}

func (j *JournaldBackend) logs(ctx context.Context, params map[string]string) (*Result, error) {
	unit, err := requireParam("journald", "logs", "unit", params)
	if err != nil {
		return nil, err
	}
	if err := validateUnit(unit); err != nil {
		return nil, err
	}

	lines := "100"
	if l := params["lines"]; l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("journald logs: invalid lines %q", l)
		}
		if n > 500 {
			n = 500
		}
		lines = strconv.Itoa(n)
	}

	args := []string{"-u", unit, "-n", lines, "--no-pager", "-o", "short-iso"}

	if since := params["since"]; since != "" {
		args = append(args, "--since", since)
	}
	if priority := params["priority"]; priority != "" {
		args = append(args, "-p", priority)
	}

	out, err := exec.CommandContext(ctx, "journalctl", args...).CombinedOutput()
	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}

func (j *JournaldBackend) tail(ctx context.Context, params map[string]string) (*Result, error) {
	unit, err := requireParam("journald", "tail", "unit", params)
	if err != nil {
		return nil, err
	}
	if err := validateUnit(unit); err != nil {
		return nil, err
	}

	lines := "50"
	if l := params["lines"]; l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("journald tail: invalid lines %q", l)
		}
		if n > 200 {
			n = 200
		}
		lines = strconv.Itoa(n)
	}

	args := []string{"-u", unit, "-n", lines, "--no-pager", "-o", "short-iso"}
	out, err := exec.CommandContext(ctx, "journalctl", args...).CombinedOutput()
	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}

func (j *JournaldBackend) boots(ctx context.Context) (*Result, error) {
	out, err := exec.CommandContext(ctx, "journalctl", "--list-boots", "--no-pager", "-o", "json").CombinedOutput()
	if err != nil {
		// --list-boots doesn't always support -o json, fall back to plain
		out, err = exec.CommandContext(ctx, "journalctl", "--list-boots", "--no-pager").CombinedOutput()
	}
	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}

func (j *JournaldBackend) diskUsage(ctx context.Context) (*Result, error) {
	out, err := exec.CommandContext(ctx, "journalctl", "--disk-usage", "--no-pager").CombinedOutput()
	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, err
}
