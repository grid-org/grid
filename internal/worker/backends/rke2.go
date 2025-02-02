package backends

import (
	"fmt"

	"github.com/bitfield/script"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type RKE2Backend struct{}

func init() {
	registerBackend("rke2", &RKE2Backend{})
}

func (r *RKE2Backend) Run(job client.Job) error {
	log.Info("Running rke2 backend", "action", job.Action, "payload", job.Payload)

	switch job.Action {
	case "download":
		// Download rke2 installer
		_, err := script.Get("https://get.rke2.io").WriteFile("/tmp/rke2.sh")
		if err != nil {
			log.Error("Error downloading rke2 installer", "error", err)
			return err
		}
	case "install":
		// Install rke2
		cmd := fmt.Sprintf("sh /tmp/rke2.sh %s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error installing rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	case "uninstall":
		// Uninstall rke2
		cmd := fmt.Sprintf("rke2-uninstall.sh %s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error uninstalling rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	case "start":
		// Enable rke2 service based on payload
		cmd := fmt.Sprintf("systemctl enable --now --no-block rke2-%s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error starting rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	case "stop":
		// Disable rke2 service based on payload
		cmd := fmt.Sprintf("systemctl disable --now --no-block rke2-%s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error stopping rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	case "restart":
		// Restart rke2 service based on payload
		cmd := fmt.Sprintf("systemctl restart rke2-%s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error restarting rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	case "kill":
		// Kill rke2 service with script
		cmd := fmt.Sprintf("rke2-killall.sh %s", job.Payload)
		out, err := script.Exec(cmd).String()
		if err != nil {
			log.Error("Error killing rke2", "error", err)
			return err
		}
		log.Debug("Job output", "output", out)
	default:
		return fmt.Errorf("Unknown action for rke2 backend")
	}

	return nil
}
