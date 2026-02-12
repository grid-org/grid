package controller

import (
	"fmt"
	"net/url"
	"time"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/scheduler"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Controller struct {
	api       *api.API
	config    *config.Config
	client    *client.Client
	server    *server.Server
	scheduler *scheduler.Scheduler
	registry  *registry.Registry
}

func New(cfg *config.Config) *Controller {
	return &Controller{
		config: cfg,
	}
}

func (c *Controller) Start() error {
	var err error

	// Create NATS server
	log.Debug("Creating NATS server")
	serverOpts := &server.Options{
		ServerName:         c.config.NATS.Name,
		Host:               c.config.NATS.Server.Host,
		Port:               c.config.NATS.Server.Port,
		JetStream:          true,
		JetStreamMaxMemory: c.config.NATS.JetStream.MaxMemory,
		JetStreamMaxStore:  c.config.NATS.JetStream.MaxStore,
		StoreDir:           c.config.NATS.JetStream.StoreDir,
		NoSigs:             true,
	}

	if c.config.NATS.Cluster.Enabled {
		serverOpts.Cluster = server.ClusterOpts{
			Name: c.config.NATS.Cluster.Name,
			Host: c.config.NATS.Cluster.Host,
			Port: c.config.NATS.Cluster.Port,
		}
		serverOpts.Routes = []*url.URL{}
		for _, route := range c.config.NATS.Cluster.Routes {
			serverOpts.Routes = append(serverOpts.Routes, &url.URL{
				Scheme: "nats",
				Host:   route,
			})
		}
	}

	if c.config.NATS.HTTP.Enabled {
		serverOpts.HTTPHost = c.config.NATS.HTTP.Host
		serverOpts.HTTPPort = c.config.NATS.HTTP.Port
	}

	c.server, err = server.NewServer(serverOpts)
	if err != nil {
		return fmt.Errorf("creating NATS server: %w", err)
	}

	// Start server
	log.Debug("Starting NATS server")
	c.server.Start()
	if !c.server.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("NATS server not ready")
	}
	defer c.server.Shutdown()

	// Connect over IPC
	log.Debug("Connecting to NATS server over IPC")
	c.client, err = client.New(c.config, nats.InProcessServer(c.server))
	if err != nil {
		return fmt.Errorf("connecting to NATS server: %w", err)
	}
	defer c.client.Close()

	// Ensure streams and KV buckets
	if err := c.ensureInfrastructure(); err != nil {
		return err
	}

	// Initialize registry and scheduler
	c.registry = registry.New(c.client)
	c.scheduler = scheduler.New(c.client, c.registry)

	log.Info("Controller started")

	if c.config.API.Enabled {
		c.api = api.New(c.config, c.client, c.scheduler)
		if err = c.api.Start(); err != nil {
			return fmt.Errorf("starting API server: %w", err)
		}
	}

	common.WaitForSignal()

	if c.api != nil {
		if err := c.api.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) ensureInfrastructure() error {
	// Commands stream: controller dispatches commands to workers
	log.Debug("Ensuring commands stream")
	if _, err := c.client.EnsureStream(jetstream.StreamConfig{
		Name:      "commands",
		Subjects:  []string{"cmd.>"},
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    1 * time.Hour,
		Replicas:  c.config.NATS.JetStream.Replicas,
	}); err != nil {
		return fmt.Errorf("ensuring commands stream: %w", err)
	}

	// Results stream: workers report results back to controller
	log.Debug("Ensuring results stream")
	if _, err := c.client.EnsureStream(jetstream.StreamConfig{
		Name:      "results",
		Subjects:  []string{"result.>"},
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    1 * time.Hour,
		Replicas:  c.config.NATS.JetStream.Replicas,
	}); err != nil {
		return fmt.Errorf("ensuring results stream: %w", err)
	}

	// KV: cluster status
	log.Debug("Ensuring cluster KV bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "cluster",
	}); err != nil {
		return fmt.Errorf("ensuring cluster bucket: %w", err)
	}
	if err := c.client.PutKV("cluster", "status", []byte("active")); err != nil {
		return fmt.Errorf("storing cluster status: %w", err)
	}

	// KV: job metadata
	log.Debug("Ensuring jobs KV bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "jobs",
	}); err != nil {
		return fmt.Errorf("ensuring jobs bucket: %w", err)
	}

	// KV: node registration
	log.Debug("Ensuring nodes KV bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "nodes",
	}); err != nil {
		return fmt.Errorf("ensuring nodes bucket: %w", err)
	}

	return nil
}
