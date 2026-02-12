package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	cfg := Defaults()

	if cfg.API.Host != "localhost" {
		t.Errorf("API.Host = %q, want localhost", cfg.API.Host)
	}
	if cfg.API.Port != 8765 {
		t.Errorf("API.Port = %d, want 8765", cfg.API.Port)
	}
	if cfg.NATS.Server.Port != 4222 {
		t.Errorf("NATS.Server.Port = %d, want 4222", cfg.NATS.Server.Port)
	}
	if cfg.NATS.JetStream.StoreDir != ".nats" {
		t.Errorf("JetStream.StoreDir = %q, want .nats", cfg.NATS.JetStream.StoreDir)
	}
	if cfg.NATS.JetStream.Replicas != 1 {
		t.Errorf("JetStream.Replicas = %d, want 1", cfg.NATS.JetStream.Replicas)
	}
	if cfg.NATS.Cluster.Name != "grid" {
		t.Errorf("Cluster.Name = %q, want grid", cfg.NATS.Cluster.Name)
	}
	if cfg.NATS.Name == "" {
		t.Error("NATS.Name should be set from hostname")
	}
}

func TestLoadConfig_ValidYAML(t *testing.T) {
	content := `
api:
  enabled: true
  host: "0.0.0.0"
  port: 9999
nats:
  name: "test-node"
  server:
    host: "0.0.0.0"
    port: 5222
worker:
  groups: ["web", "prod"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing config: %v", err)
	}

	cfg := LoadConfig(path)

	if !cfg.API.Enabled {
		t.Error("API.Enabled should be true")
	}
	if cfg.API.Port != 9999 {
		t.Errorf("API.Port = %d, want 9999", cfg.API.Port)
	}
	if cfg.NATS.Name != "test-node" {
		t.Errorf("NATS.Name = %q, want test-node", cfg.NATS.Name)
	}
	if cfg.NATS.Server.Port != 5222 {
		t.Errorf("NATS.Server.Port = %d, want 5222", cfg.NATS.Server.Port)
	}
	if len(cfg.Worker.Groups) != 2 {
		t.Errorf("Worker.Groups len = %d, want 2", len(cfg.Worker.Groups))
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	cfg := LoadConfig("/nonexistent/path/config.yaml")

	// Should fall back to defaults
	if cfg.API.Port != 8765 {
		t.Errorf("API.Port = %d, want 8765 (default)", cfg.API.Port)
	}
	if cfg.NATS.Server.Port != 4222 {
		t.Errorf("NATS.Server.Port = %d, want 4222 (default)", cfg.NATS.Server.Port)
	}
}

func TestFixName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"valid-name", "valid-name"},
		{"has spaces", "has_spaces"},
		{"dots.in.name", "dots_in_name"},
		{"stars*here", "stars_here"},
		{"greater>than", "greater_than"},
		{"path/sep", "path_sep"},
		{"back\\slash", "back_slash"},
		{"combo .*/\\>", "combo______"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := fixName(tt.input)
			if got != tt.expected {
				t.Errorf("fixName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
