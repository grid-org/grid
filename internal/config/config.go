package config

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
	"github.com/urfave/cli/v2"
)

func LoadConfig(configFile string) *Config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Errorf("Error reading config file: %v; loading defaults", err)
		return Defaults()
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		log.Errorf("Error parsing config file: %v; loading defaults", err)
		return Defaults()
	}

	if cfg.NATS.Name == "" {
		log.Warn("No node name provided in config file; using hostname")
		// Set to the hostname if not provided
		hostname := getHostname()
		cfg.NATS.Name = hostname
	}

	if cfg.API.Host == "" {
		cfg.API.Host = "localhost"
	}

	if cfg.NATS.URL == "" {
		cfg.NATS.URL = "nats://localhost:4222"
	}

	return &cfg
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname: %v; using localhost", err)
		return "localhost"
	}
	return hostname
}

func AppFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yaml",
			Usage:   "Load configuration from `FILE`",
			EnvVars: []string{"GRID_CONFIG"},
		},
		&cli.StringFlag{
			Name: "nats-url",
			Usage: "NATS server URL",
			Value: "nats://localhost:4222",
			EnvVars: []string{"GRID_URL"},
		},
	}
}

func Defaults() *Config {
	return &Config{
		API: APIConfig{
			Host: "localhost",
			Port: 8765,
		},
		NATS: NATSConfig{
			URL:  "nats://localhost:4222",
			Name: getHostname(),
		},
	}
}
