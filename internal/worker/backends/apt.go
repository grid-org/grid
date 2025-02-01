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

func (a *APTBackend) Run(req client.Request) error {
	log.Info("Running apt backend", "payload", req.Payload)
	return nil
}
