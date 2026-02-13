package testutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/controller"
	"github.com/grid-org/grid/internal/models"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// TestEnv provides an embedded NATS server and client for tests.
// Each test (or test group) should create its own TestEnv for isolation.
type TestEnv struct {
	Server *server.Server
	Client *client.Client
	Config *config.Config
}

// NewTestEnv starts an embedded NATS server on a random port, connects a
// client over IPC, and creates all required streams and KV buckets.
// Cleanup is registered automatically via t.Cleanup.
func NewTestEnv(t *testing.T) *TestEnv {
	t.Helper()

	cfg := config.Defaults()
	cfg.NATS.Name = fmt.Sprintf("test-%s", t.Name())
	cfg.NATS.Server.Port = -1 // random port
	cfg.NATS.JetStream.Replicas = 1

	opts := &server.Options{
		ServerName: cfg.NATS.Name,
		Host:       "127.0.0.1",
		Port:       -1, // random
		JetStream:  true,
		StoreDir:   t.TempDir(),
		NoSigs:     true,
		NoLog:      true,
	}

	srv, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("creating test NATS server: %v", err)
	}
	srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		t.Fatal("test NATS server not ready")
	}

	// Update config with actual URL for any non-IPC connections (e.g. workers)
	cfg.NATS.Client.URLS = []string{srv.ClientURL()}

	c, err := client.New(cfg, nats.InProcessServer(srv))
	if err != nil {
		srv.Shutdown()
		t.Fatalf("connecting to test NATS: %v", err)
	}

	// Create all infrastructure (streams + KV)
	if err := controller.EnsureInfrastructure(c, 1); err != nil {
		c.Close()
		srv.Shutdown()
		t.Fatalf("ensuring infrastructure: %v", err)
	}

	env := &TestEnv{
		Server: srv,
		Client: c,
		Config: cfg,
	}

	t.Cleanup(func() {
		c.Close()
		srv.Shutdown()
	})

	return env
}

// RegisterNodes stores node registrations in the nodes KV bucket.
func (e *TestEnv) RegisterNodes(t *testing.T, nodes ...models.NodeInfo) {
	t.Helper()
	for _, n := range nodes {
		if err := e.Client.PutNode(n); err != nil {
			t.Fatalf("registering node %s: %v", n.ID, err)
		}
	}
}

// OnlineNode returns a NodeInfo suitable for testing with the given ID and groups.
func OnlineNode(id string, groups ...string) models.NodeInfo {
	return models.NodeInfo{
		ID:       id,
		Hostname: id,
		Groups:   groups,
		Backends: []string{"test", "ping"},
		Status:   "online",
		LastSeen: time.Now().UTC(),
	}
}

// RegisterControllers stores controller registrations in the controllers KV bucket.
func (e *TestEnv) RegisterControllers(t *testing.T, controllers ...models.ControllerInfo) {
	t.Helper()
	for _, ctrl := range controllers {
		if err := e.Client.PutController(ctrl); err != nil {
			t.Fatalf("registering controller %s: %v", ctrl.ID, err)
		}
	}
}

// OnlineController returns a ControllerInfo suitable for testing with the given ID.
func OnlineController(id string) models.ControllerInfo {
	return models.ControllerInfo{
		ID:        id,
		Hostname:  id,
		Status:    "online",
		LastSeen:  time.Now().UTC(),
		StartedAt: time.Now().UTC(),
	}
}

// WaitFor polls fn every 50ms until it returns true or timeout elapses.
func WaitFor(t *testing.T, timeout time.Duration, fn func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for: %s", msg)
}
