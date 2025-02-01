package backends

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
)

type RKE2Backend struct{}

func init() {
	log.Info("Registering rke2 backend")
	// Register the rke2 backend
	registerBackend("rke2", &RKE2Backend{})
}

func (r *RKE2Backend) Run(req client.Request) error {
	return nil
}
