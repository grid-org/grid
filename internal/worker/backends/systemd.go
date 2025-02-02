package backends

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type SystemdBackend struct{}

func init() {
	registerBackend("systemd", &SystemdBackend{})
}

func (s *SystemdBackend) Run(job client.Job) error {
	log.Info("Running systemd backend", "action", job.Action, "payload", job.Payload)
	return nil
}
