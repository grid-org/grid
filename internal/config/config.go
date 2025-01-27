package config

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
)

func LoadConfig(configFile string) *Config {
	data, err := os.ReadFile(configFile)
	if err != nil {
		log.Errorf("Error reading config file: %v; loading defaults", err)
		return Defaults()
	}

	cfg := Defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		log.Errorf("Error parsing config file: %v; loading defaults", err)
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
			URLS:  []string{"nats://localhost:4222"},
			Name: getHostname(),
		},
	}
}
