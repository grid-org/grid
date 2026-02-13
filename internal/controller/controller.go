package controller

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/scheduler"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Controller struct {
	api          *api.API
	config       *config.Config
	client       *client.Client
	server       *server.Server
	scheduler    *scheduler.Scheduler
	registry     *registry.Registry
	controllerID string
	startedAt    time.Time
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

	// Initialize controller identity
	c.controllerID = c.config.NATS.Name
	c.startedAt = time.Now().UTC()

	// Register this controller
	if err := c.registerController(); err != nil {
		return fmt.Errorf("registering controller: %w", err)
	}

	// Start heartbeat
	hbCtx, hbCancel := context.WithCancel(context.Background())
	defer hbCancel()
	go c.heartbeat(hbCtx)

	// Initialize registry and scheduler
	c.registry = registry.New(c.client)
	c.scheduler = scheduler.New(c.client, c.registry, c.config.Scheduler, c.controllerID)

	// Start the scheduler's request-pulling loop
	schedCtx, schedCancel := context.WithCancel(context.Background())
	defer schedCancel()

	if err := c.scheduler.Start(schedCtx); err != nil {
		return fmt.Errorf("starting scheduler: %w", err)
	}

	// Start stale job recovery loop
	staleThreshold := 60 * time.Second
	if c.config.Controller.StaleThreshold != "" {
		if d, err := time.ParseDuration(c.config.Controller.StaleThreshold); err == nil {
			staleThreshold = d
		} else {
			log.Warn("Invalid stale_threshold, using default", "value", c.config.Controller.StaleThreshold)
		}
	}
	c.scheduler.StartRecovery(schedCtx, staleThreshold)

	log.Info("Controller started", "id", c.controllerID)

	if c.config.API.Enabled {
		c.api = api.New(c.config, c.client, c.scheduler)
		c.api.SetNATSServer(c.server)
		if err = c.api.Start(); err != nil {
			return fmt.Errorf("starting API server: %w", err)
		}
	}

	common.WaitForSignal()

	schedCancel()
	hbCancel()
	c.deregisterController()

	if c.api != nil {
		if err := c.api.Stop(); err != nil {
			return err
		}
	}
	return nil
}

func (c *Controller) ensureInfrastructure() error {
	return EnsureInfrastructure(c.client, c.config.NATS.JetStream.Replicas)
}

// EnsureInfrastructure creates the required streams and KV buckets.
// Exported so test helpers can reuse it without starting a full controller.
func EnsureInfrastructure(c *client.Client, replicas int) error {
	// Commands stream: controller dispatches commands to workers
	if _, err := c.EnsureStream(jetstream.StreamConfig{
		Name:      "commands",
		Subjects:  []string{"cmd.>"},
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    1 * time.Hour,
		Replicas:  replicas,
	}); err != nil {
		return fmt.Errorf("ensuring commands stream: %w", err)
	}

	// Results stream: workers report results back to controller
	if _, err := c.EnsureStream(jetstream.StreamConfig{
		Name:      "results",
		Subjects:  []string{"result.>"},
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    1 * time.Hour,
		Replicas:  replicas,
	}); err != nil {
		return fmt.Errorf("ensuring results stream: %w", err)
	}

	// Requests stream: API enqueues jobs, scheduler pulls them (work queue)
	if _, err := c.EnsureStream(jetstream.StreamConfig{
		Name:      "requests",
		Subjects:  []string{"request.>"},
		Retention: jetstream.WorkQueuePolicy,
		Discard:   jetstream.DiscardOld,
		MaxAge:    1 * time.Hour,
		Replicas:  replicas,
	}); err != nil {
		return fmt.Errorf("ensuring requests stream: %w", err)
	}

	// KV: cluster status
	if _, err := c.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "cluster",
	}); err != nil {
		return fmt.Errorf("ensuring cluster bucket: %w", err)
	}
	if err := c.PutKV("cluster", "status", []byte("active")); err != nil {
		return fmt.Errorf("storing cluster status: %w", err)
	}

	// KV: job metadata
	if _, err := c.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "jobs",
	}); err != nil {
		return fmt.Errorf("ensuring jobs bucket: %w", err)
	}

	// KV: node registration
	if _, err := c.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "nodes",
	}); err != nil {
		return fmt.Errorf("ensuring nodes bucket: %w", err)
	}

	// KV: controller registration
	if _, err := c.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "controllers",
	}); err != nil {
		return fmt.Errorf("ensuring controllers bucket: %w", err)
	}

	return nil
}

func (c *Controller) registerController() error {
	hostname, _ := os.Hostname()
	info := models.ControllerInfo{
		ID:        c.controllerID,
		Hostname:  hostname,
		Status:    "online",
		LastSeen:  time.Now().UTC(),
		StartedAt: c.startedAt,
	}
	return c.client.PutController(info)
}

func (c *Controller) deregisterController() {
	info := models.ControllerInfo{
		ID:        c.controllerID,
		Hostname:  "",
		Status:    "offline",
		LastSeen:  time.Now().UTC(),
		StartedAt: c.startedAt,
	}
	if err := c.client.PutController(info); err != nil {
		log.Error("Failed to deregister controller", "error", err)
	}
}

func (c *Controller) heartbeat(ctx context.Context) {
	interval := 15 * time.Second
	if c.config.Controller.HeartbeatInterval != "" {
		if d, err := time.ParseDuration(c.config.Controller.HeartbeatInterval); err == nil {
			interval = d
		} else {
			log.Warn("Invalid controller heartbeat_interval, using default", "value", c.config.Controller.HeartbeatInterval)
		}
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.registerController(); err != nil {
				log.Error("Controller heartbeat failed", "error", err)
			}
		}
	}
}
