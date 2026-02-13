package backends

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxOutputBytes is the maximum size of output returned from any backend action.
const MaxOutputBytes = 256 * 1024 // 256 KB

// DefaultAllowedPaths is the default set of paths that file-touching backends may access.
var DefaultAllowedPaths = []string{"/etc", "/var/log", "/tmp/grid", "/opt"}

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

// PathAware is an optional interface for backends that need path validation.
type PathAware interface {
	SetAllowedPaths(paths []string)
}

type Backends struct {
	backends map[string]Backend
}

// New creates a new Backends registry. allowedPaths controls which filesystem
// paths are accessible to file-touching backends. Pass nil for defaults.
func New(allowedPaths []string) *Backends {
	if allowedPaths == nil {
		allowedPaths = DefaultAllowedPaths
	}
	b := &Backends{
		backends: initBackends,
	}
	for _, backend := range b.backends {
		if pa, ok := backend.(PathAware); ok {
			pa.SetAllowedPaths(allowedPaths)
		}
	}
	return b
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

// --- Shared helpers ---

// truncateOutput truncates s to MaxOutputBytes, appending a truncation notice.
func truncateOutput(s string) string {
	if len(s) <= MaxOutputBytes {
		return s
	}
	return s[:MaxOutputBytes] + "\n... [output truncated]"
}

// ValidatePath checks that path is under one of the allowed prefixes.
// It resolves symlinks and rejects path traversal.
func ValidatePath(path string, allowedPaths []string) error {
	if path == "" {
		return fmt.Errorf("path is required")
	}

	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal not allowed: %s", path)
	}

	// Resolve symlinks if the path exists
	resolved := cleaned
	if _, err := os.Lstat(cleaned); err == nil {
		r, err := filepath.EvalSymlinks(cleaned)
		if err != nil {
			return fmt.Errorf("resolving symlinks: %w", err)
		}
		resolved = r
	}

	for _, allowed := range allowedPaths {
		allowedClean := filepath.Clean(allowed)
		if resolved == allowedClean || strings.HasPrefix(resolved, allowedClean+"/") {
			return nil
		}
		// Also check the cleaned (pre-symlink) path for new files
		if cleaned == allowedClean || strings.HasPrefix(cleaned, allowedClean+"/") {
			return nil
		}
	}

	return fmt.Errorf("path %s is not under any allowed path %v", path, allowedPaths)
}

// requireParam extracts a required parameter, returning a descriptive error if missing.
func requireParam(backend, action, key string, params map[string]string) (string, error) {
	v := params[key]
	if v == "" {
		return "", fmt.Errorf("%s %s: missing required param %q", backend, action, key)
	}
	return v, nil
}
