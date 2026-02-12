package backends

import "context"

var initBackends = make(map[string]Backend)

// Result holds the output of a backend action execution.
type Result struct {
	Output string
}

// Backend defines the interface that all execution backends must implement.
type Backend interface {
	// Run executes an action with structured parameters.
	// Context carries cancellation/timeout from the scheduler.
	Run(ctx context.Context, action string, params map[string]string) (*Result, error)

	// Actions returns the set of valid action names for this backend.
	Actions() []string
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

// List returns the names of all registered backends.
func (b *Backends) List() []string {
	names := make([]string, 0, len(b.backends))
	for name := range b.backends {
		names = append(names, name)
	}
	return names
}

func registerBackend(name string, backend Backend) {
	initBackends[name] = backend
}
