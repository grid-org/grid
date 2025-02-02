package backends

import (
	"fmt"
	"github.com/bitfield/script"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type HostBackend struct{}

func init() {
	registerBackend("host", &HostBackend{})
}

func (h *HostBackend) Run(job client.Job) error {
	log.Info("Running host backend", "action", job.Action, "payload", job.Payload)

	command := fmt.Sprintf("%s %s", job.Action, job.Payload)

	out, err := script.Exec(command).String()
	if err != nil {
		log.Error("Error running command", "error", err)
		return err
	}
	log.Info("Command output", "output", out)

	return nil
}
