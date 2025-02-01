package backends

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type SystemdBackend struct{}

func init() {
	log.Info("Registering systemd backend")
	// Register the systemd backend
	registerBackend("systemd", &SystemdBackend{})
}

func (s *SystemdBackend) Run(job client.Job) error {
	return nil
}
