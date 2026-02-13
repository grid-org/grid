package backends

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestConfigBackend(t *testing.T) (*ConfigBackend, string) {
	t.Helper()
	dir := t.TempDir()
	b := &ConfigBackend{allowedPaths: []string{dir}}
	return b, dir
}

func TestConfigBackend_Actions(t *testing.T) {
	b := &ConfigBackend{}
	want := map[string]bool{"get": true, "set": true, "validate": true, "diff": true}
	for _, a := range b.Actions() {
		if !want[a] {
			t.Errorf("unexpected action %q", a)
		}
		delete(want, a)
	}
	for a := range want {
		t.Errorf("missing action %q", a)
	}
}

func TestConfigBackend_GetSetYAML(t *testing.T) {
	b, dir := newTestConfigBackend(t)
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte("server:\n  host: localhost\n  port: 8080\n"), 0644)

	// Get nested key
	result, err := b.Run(context.Background(), "get", map[string]string{
		"path": path,
		"key":  "server.host",
	})
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if result.Output != "localhost" {
		t.Errorf("expected localhost, got %q", result.Output)
	}

	// Set key
	result, err = b.Run(context.Background(), "set", map[string]string{
		"path":  path,
		"key":   "server.host",
		"value": "0.0.0.0",
	})
	if err != nil {
		t.Fatalf("set error: %v", err)
	}
	if !strings.Contains(result.Output, "set") {
		t.Errorf("expected 'set' in output, got %q", result.Output)
	}

	// Verify backup was created
	if _, err := os.Stat(path + ".bak"); os.IsNotExist(err) {
		t.Error("expected backup file")
	}

	// Verify new value
	result, err = b.Run(context.Background(), "get", map[string]string{
		"path": path,
		"key":  "server.host",
	})
	if err != nil {
		t.Fatalf("get error after set: %v", err)
	}
	if result.Output != "0.0.0.0" {
		t.Errorf("expected 0.0.0.0, got %q", result.Output)
	}
}

func TestConfigBackend_GetSetJSON(t *testing.T) {
	b, dir := newTestConfigBackend(t)
	path := filepath.Join(dir, "test.json")
	os.WriteFile(path, []byte(`{"name":"grid","version":"1.0"}`), 0644)

	result, err := b.Run(context.Background(), "get", map[string]string{
		"path": path,
		"key":  "name",
	})
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if result.Output != "grid" {
		t.Errorf("expected grid, got %q", result.Output)
	}
}

func TestConfigBackend_GetSetINI(t *testing.T) {
	b, dir := newTestConfigBackend(t)
	path := filepath.Join(dir, "test.ini")
	os.WriteFile(path, []byte("[database]\nhost = localhost\nport = 5432\n"), 0644)

	result, err := b.Run(context.Background(), "get", map[string]string{
		"path": path,
		"key":  "database.host",
	})
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if result.Output != "localhost" {
		t.Errorf("expected localhost, got %q", result.Output)
	}
}

func TestConfigBackend_Validate(t *testing.T) {
	b, dir := newTestConfigBackend(t)

	// Valid YAML
	validPath := filepath.Join(dir, "valid.yaml")
	os.WriteFile(validPath, []byte("key: value\n"), 0644)

	result, err := b.Run(context.Background(), "validate", map[string]string{
		"path":   validPath,
		"format": "yaml",
	})
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	var data map[string]any
	json.Unmarshal([]byte(result.Output), &data)
	if !data["valid"].(bool) {
		t.Error("expected valid=true")
	}

	// Invalid JSON
	invalidPath := filepath.Join(dir, "invalid.json")
	os.WriteFile(invalidPath, []byte("{broken"), 0644)

	result, err = b.Run(context.Background(), "validate", map[string]string{
		"path":   invalidPath,
		"format": "json",
	})
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	json.Unmarshal([]byte(result.Output), &data)
	if data["valid"].(bool) {
		t.Error("expected valid=false for broken JSON")
	}
}

func TestConfigBackend_Diff(t *testing.T) {
	b, dir := newTestConfigBackend(t)
	path := filepath.Join(dir, "diff.txt")
	os.WriteFile(path, []byte("line1\nline2\nline3\n"), 0644)

	result, err := b.Run(context.Background(), "diff", map[string]string{
		"path":    path,
		"content": "line1\nLINE2\nline3\n",
	})
	if err != nil {
		t.Fatalf("diff error: %v", err)
	}
	if !strings.Contains(result.Output, "-line2") || !strings.Contains(result.Output, "+LINE2") {
		t.Errorf("expected diff to show changes, got %q", result.Output)
	}
}

func TestConfigBackend_PathValidation(t *testing.T) {
	b, _ := newTestConfigBackend(t)
	_, err := b.Run(context.Background(), "get", map[string]string{
		"path": "/etc/passwd",
		"key":  "root",
	})
	if err == nil {
		t.Fatal("expected path validation error")
	}
}

func TestConfigBackend_MissingKey(t *testing.T) {
	b, dir := newTestConfigBackend(t)
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte("key: value\n"), 0644)

	_, err := b.Run(context.Background(), "get", map[string]string{
		"path": path,
		"key":  "nonexistent",
	})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestConfigBackend_UnknownAction(t *testing.T) {
	b := &ConfigBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
