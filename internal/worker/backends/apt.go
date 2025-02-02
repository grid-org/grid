package backends

import (
	"fmt"
	"github.com/bitfield/script"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type APTBackend struct{}

func init() {
	registerBackend("apt", &APTBackend{})
}

func (a *APTBackend) Run(job client.Job) error {
	log.Info("Running apt backend", "action", job.Action, "payload", job.Payload)

	// Check if the action is "install"
	switch job.Action {
	case "install":
		// Install the package
		cmd := fmt.Sprintf("apt install -y %s", job.Payload)
		err := script.Exec(cmd).Wait()
		if err != nil {
			log.Error("Error installing package", "error", err)
			return err
		}
	default:
		return fmt.Errorf("Unknown action for apt backend")
	}

	return nil
}
