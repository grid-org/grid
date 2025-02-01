package backends

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type HostBackend struct{}

func init() {
	log.Info("Registering host backend")
	// Register the host backend
	registerBackend("host", &HostBackend{})
}

func (h *HostBackend) Run(job client.Job) error {
	return nil
}
