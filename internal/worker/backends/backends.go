package backends

import "github.com/grid-org/grid/internal/client"

var initBackends = make(map[string]Backend)

type Backend interface {
	Run(client.Job) error
}

type Backends struct {
	backends map[string]Backend
}

func New() *Backends {
	return &Backends{
		backends: initBackends,
	}
}

func (b *Backends) Get(name string) (Backend, bool) {
	backend, ok := b.backends[name]
	return backend, ok
}

func registerBackend(name string, backend Backend) {
	initBackends[name] = backend
}
