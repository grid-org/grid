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

func (s *SystemdBackend) Run(req client.Request) error {
	return nil
}
