package controller

import (
	"fmt"
	"net/url"
	"time"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Controller struct {
	api    *api.API
	config *config.Config
	client *client.Client
	server *server.Server
}

func New(cfg *config.Config) *Controller {
	return &Controller{
		config: cfg,
	}
}

func (c *Controller) Start() error {
	var err error

	// Create server
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
		return fmt.Errorf("Error creating NATS server: %w", err)
	}

	// Start server
	log.Debug("Starting NATS server")
	c.server.Start()
	if !c.server.ReadyForConnections(10 * time.Second) {
		return fmt.Errorf("Error starting NATS server: %w", err)
	}
	defer c.server.Shutdown()

	// Connect to server
	log.Debug("Connecting to NATS server over IPC")
	c.client, err = client.New(c.config, nats.InProcessServer(c.server))
	if err != nil {
		return fmt.Errorf("Error connecting to NATS server: %w", err)
	}
	defer c.client.Close()

	// Ensure streams
	log.Debug("Ensuring jobs stream")
	if _, err := c.client.EnsureStream(jetstream.StreamConfig{
		Name:      "jobs",
		Subjects:  []string{"job.*.>"},
		Retention: jetstream.LimitsPolicy,
		Discard:   jetstream.DiscardOld,
		Replicas:  c.config.NATS.JetStream.Replicas,
	}); err != nil {
		return fmt.Errorf("Error ensuring stream: %w", err)
	}

	// Create cluster bucket
	log.Debug("Creating key-value bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "cluster",
	}); err != nil {
		return fmt.Errorf("Error creating key-value bucket: %w", err)
	}

	// Store cluster status
	log.Debug("Storing cluster status")
	if err := c.client.PutKV("cluster", "status", []byte("active")); err != nil {
		return fmt.Errorf("Error storing cluster status: %w", err)
	}

	// Create jobs bucket
	log.Debug("Ensuring jobs bucket")
	if _, err := c.client.EnsureKV(jetstream.KeyValueConfig{
		Bucket: "jobs",
	}); err != nil {
		return fmt.Errorf("Error ensuring jobs bucket: %w", err)
	}

	log.Info("Controller started")

	if c.config.API.Enabled {
		c.api = api.New(c.config, c.client)
		err = c.api.Start()
		if err != nil {
			return fmt.Errorf("Error starting API server: %w", err)
		}
	}

	common.WaitForSignal()

	if err := c.api.Stop(); err != nil {
		return err
	}
	return nil
}
