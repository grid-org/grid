package backends

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/log"
)

type SystemdBackend struct{}

func init() {
	registerBackend("systemd", &SystemdBackend{})
}

func (s *SystemdBackend) Actions() []string {
	return []string{"start", "stop", "restart", "enable", "disable", "status"}
}

func (s *SystemdBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	unit := params["unit"]
	if unit == "" {
		return nil, fmt.Errorf("systemd %s: missing required param 'unit'", action)
	}

	log.Info("Running systemd backend", "action", action, "unit", unit)

	var args []string
	switch action {
	case "start":
		args = []string{"start", unit}
	case "stop":
		args = []string{"stop", unit}
	case "restart":
		args = []string{"restart", unit}
	case "enable":
		args = []string{"enable", "--now", unit}
	case "disable":
		args = []string{"disable", "--now", unit}
	case "status":
		args = []string{"status", unit}
	default:
		return nil, fmt.Errorf("systemd: unknown action %q", action)
	}

	out, err := exec.CommandContext(ctx, "systemctl", args...).CombinedOutput()
	return &Result{Output: string(out)}, err
}
