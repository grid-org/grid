package config

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
)

type APIConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type NATSConfig struct {
	URLS []string `yaml:"urls"`
	Name string   `yaml:"name"`
}

type Config struct {
	API  APIConfig  `yaml:"api"`
	NATS NATSConfig `yaml:"nats"`
}

func LoadConfig(configFile string) *Config {
	cfg := Defaults()

	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Warn("Error reading config file: %v; loading defaults", "error", err)
		return cfg
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Warn("Error parsing config file: %v; loading defaults", "error", err)
		return cfg
	}

	return cfg
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		log.Errorf("Error getting hostname: %v; using localhost", err)
		return "localhost"
	}
	return hostname
}

func Defaults() *Config {
	return &Config{
		API: APIConfig{
			Host: "localhost",
			Port: 8765,
		},
		NATS: NATSConfig{
			URLS: []string{"nats://localhost:4222"},
			Name: getHostname(),
		},
	}
}
