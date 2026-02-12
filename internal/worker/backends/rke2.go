package backends

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/charmbracelet/log"
)

type RKE2Backend struct{}

func init() {
	registerBackend("rke2", &RKE2Backend{})
}

func (r *RKE2Backend) Actions() []string {
	return []string{"download", "install", "uninstall", "start", "stop", "restart", "kill"}
}

func (r *RKE2Backend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	log.Info("Running rke2 backend", "action", action, "params", params)

	mode := params["mode"] // "server" or "agent"

	switch action {
	case "download":
		return r.download(ctx)
	case "install":
		flags := params["flags"]
		return r.exec(ctx, "sh", "/tmp/rke2.sh", flags)
	case "uninstall":
		return r.exec(ctx, "rke2-uninstall.sh")
	case "start":
		if mode == "" {
			return nil, fmt.Errorf("rke2 start: missing required param 'mode' (server|agent)")
		}
		return r.exec(ctx, "systemctl", "enable", "--now", "--no-block", fmt.Sprintf("rke2-%s", mode))
	case "stop":
		if mode == "" {
			return nil, fmt.Errorf("rke2 stop: missing required param 'mode' (server|agent)")
		}
		return r.exec(ctx, "systemctl", "disable", "--now", "--no-block", fmt.Sprintf("rke2-%s", mode))
	case "restart":
		if mode == "" {
			return nil, fmt.Errorf("rke2 restart: missing required param 'mode' (server|agent)")
		}
		return r.exec(ctx, "systemctl", "restart", fmt.Sprintf("rke2-%s", mode))
	case "kill":
		return r.exec(ctx, "rke2-killall.sh")
	default:
		return nil, fmt.Errorf("rke2: unknown action %q", action)
	}
}

func (r *RKE2Backend) download(ctx context.Context) (*Result, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://get.rke2.io", nil)
	if err != nil {
		return nil, fmt.Errorf("rke2 download: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rke2 download: %w", err)
	}
	defer resp.Body.Close()

	f, err := os.Create("/tmp/rke2.sh")
	if err != nil {
		return nil, fmt.Errorf("rke2 download: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return nil, fmt.Errorf("rke2 download: %w", err)
	}

	return &Result{Output: "downloaded installer to /tmp/rke2.sh"}, nil
}

func (r *RKE2Backend) exec(ctx context.Context, name string, args ...string) (*Result, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return &Result{Output: string(out)}, err
}
