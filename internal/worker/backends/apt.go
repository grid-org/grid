package backends

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type APTBackend struct{}

func init() {
	log.Info("Registering apt backend")
	// Register the apt backend
	registerBackend("apt", &APTBackend{})
}

func (a *APTBackend) Run(job client.Job) error {
	log.Info("Running apt backend", "action", job.Action, "payload", job.Payload)
	return nil
}
