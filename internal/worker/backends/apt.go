package backends

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/charmbracelet/log"
)

type APTBackend struct{}

func init() {
	registerBackend("apt", &APTBackend{})
}

func (a *APTBackend) Actions() []string {
	return []string{"install", "remove", "update", "upgrade"}
}

func (a *APTBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	log.Info("Running apt backend", "action", action, "params", params)

	var cmd *exec.Cmd
	switch action {
	case "install":
		pkg := params["package"]
		if pkg == "" {
			return nil, fmt.Errorf("apt install: missing required param 'package'")
		}
		cmd = exec.CommandContext(ctx, "apt-get", "install", "-y", pkg)
	case "remove":
		pkg := params["package"]
		if pkg == "" {
			return nil, fmt.Errorf("apt remove: missing required param 'package'")
		}
		cmd = exec.CommandContext(ctx, "apt-get", "remove", "-y", pkg)
	case "update":
		cmd = exec.CommandContext(ctx, "apt-get", "update")
	case "upgrade":
		cmd = exec.CommandContext(ctx, "apt-get", "upgrade", "-y")
	default:
		return nil, fmt.Errorf("apt: unknown action %q", action)
	}

	out, err := cmd.CombinedOutput()
	return &Result{Output: string(out)}, err
}
