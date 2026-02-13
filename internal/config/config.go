package config

import (
	"os"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
)

type APIConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

type ClientConfig struct {
	URLS []string `yaml:"urls"`
}

type ClusterConfig struct {
	Enabled bool     `yaml:"enabled"`
	Name    string   `yaml:"name"`
	Host    string   `yaml:"host"`
	Port    int      `yaml:"port"`
	Routes  []string `yaml:"routes"`
}

type JetStreamConfig struct {
	Replicas  int    `yaml:"replicas"`
	StoreDir  string `yaml:"store"`
	MaxStore  int64  `yaml:"max_store"`
	MaxMemory int64  `yaml:"max_memory"`
	Domain    string `yaml:"domain"`
}

type HTTPConfig struct {
	Enabled bool   `yaml:"enabled"`
	Host    string `yaml:"host"`
	Port    int    `yaml:"port"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type NATSConfig struct {
	Name      string          `yaml:"name"`
	Client    ClientConfig    `yaml:"client"`
	Cluster   ClusterConfig   `yaml:"cluster"`
	JetStream JetStreamConfig `yaml:"jetstream"`
	HTTP      HTTPConfig      `yaml:"http"`
	Server    ServerConfig    `yaml:"server"`
}

type WorkerConfig struct {
	Groups            []string `yaml:"groups"`
	HeartbeatInterval string   `yaml:"heartbeat_interval"` // e.g. "30s" (default)
	InactiveThreshold string   `yaml:"inactive_threshold"` // e.g. "10m" (default)
	AllowedPaths      []string `yaml:"allowed_paths"`      // filesystem paths backends may access
}

type SchedulerConfig struct {
	MaxConcurrent int `yaml:"max_concurrent"` // max jobs running in parallel (default: 5)
	MaxPending    int `yaml:"max_pending"`    // max jobs waiting in queue (default: 100)
}

type ControllerConfig struct {
	HeartbeatInterval string `yaml:"heartbeat_interval"` // e.g. "15s" (default)
	StaleThreshold    string `yaml:"stale_threshold"`    // e.g. "60s" (default)
}

type Config struct {
	API        APIConfig        `yaml:"api"`
	NATS       NATSConfig       `yaml:"nats"`
	Worker     WorkerConfig     `yaml:"worker"`
	Scheduler  SchedulerConfig  `yaml:"scheduler"`
	Controller ControllerConfig `yaml:"controller"`
}

func LoadConfig(configFile string) *Config {
	cfg := Defaults()

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Warn("Error reading config file; loading defaults", "error", err)
		return cfg
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Warn("Error parsing config file; loading defaults", "error", err)
		return cfg
	}

	return cfg
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Warn("Error getting hostname; using localhost", "error", err)
		return "localhost"
	}
	return fixName(hostname)
}

func fixName(name string) string {
	// Various names can't have whitespace . * > or path separators
	// https://docs.nats.io/nats-concepts/jetstream/streams#configuration
	// https://docs.nats.io/nats-concepts/jetstream/consumers#consumer-names
	// https://docs.nats.io/nats-concepts/subjects#characters-allowed-and-recommended-for-subject-names

	replacer := strings.NewReplacer(
		" ", "_",
		".", "_",
		"*", "_",
		">", "_",
		"/", "_",
		"\\", "_",
	)

	// Replace all invalid characters with "_"
	return replacer.Replace(name)
}

func Defaults() *Config {
	return &Config{
		API: APIConfig{
			Host: "localhost",
			Port: 8765,
		},
		NATS: NATSConfig{
			Name: getHostname(),
			Client: ClientConfig{
				URLS: []string{"nats://localhost:4222"},
			},
			Cluster: ClusterConfig{
				Name: "grid",
			},
			JetStream: JetStreamConfig{
				StoreDir: ".nats",
				Replicas: 1,
			},
			Server: ServerConfig{
				Host: "localhost",
				Port: 4222,
			},
		},
		Scheduler: SchedulerConfig{
			MaxConcurrent: 5,
			MaxPending:    100,
		},
		Controller: ControllerConfig{
			HeartbeatInterval: "15s",
			StaleThreshold:    "60s",
		},
	}
}
