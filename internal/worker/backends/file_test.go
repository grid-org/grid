package backends

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFileBackend(t *testing.T) (*FileBackend, string) {
	t.Helper()
	dir := t.TempDir()
	b := &FileBackend{allowedPaths: []string{dir}}
	return b, dir
}

func TestFileBackend_Actions(t *testing.T) {
	b := &FileBackend{}
	want := map[string]bool{
		"read": true, "write": true, "stat": true, "exists": true,
		"mkdir": true, "delete": true, "checksum": true,
	}
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

func TestFileBackend_WriteAndRead(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "test.txt")

	// Write
	result, err := b.Run(context.Background(), "write", map[string]string{
		"path":    path,
		"content": "line1\nline2\nline3",
	})
	if err != nil {
		t.Fatalf("write error: %v", err)
	}
	if !strings.Contains(result.Output, "wrote") {
		t.Errorf("expected 'wrote' in output, got %q", result.Output)
	}

	// Read (tail, all lines)
	result, err = b.Run(context.Background(), "read", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if result.Output != "line1\nline2\nline3" {
		t.Errorf("read output = %q", result.Output)
	}

	// Read with lines limit
	result, err = b.Run(context.Background(), "read", map[string]string{"path": path, "lines": "2"})
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if result.Output != "line2\nline3" {
		t.Errorf("read tail 2 = %q, want %q", result.Output, "line2\nline3")
	}
}

func TestFileBackend_Stat(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "stat.txt")
	os.WriteFile(path, []byte("hello"), 0644)

	result, err := b.Run(context.Background(), "stat", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("stat error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if size, ok := data["size"].(float64); !ok || size != 5 {
		t.Errorf("expected size=5, got %v", data["size"])
	}
}

func TestFileBackend_Exists(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "exists.txt")

	// Does not exist
	result, err := b.Run(context.Background(), "exists", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("exists error: %v", err)
	}
	if result.Output != "false" {
		t.Errorf("expected false, got %q", result.Output)
	}

	// Create it
	os.WriteFile(path, []byte("x"), 0644)
	result, err = b.Run(context.Background(), "exists", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("exists error: %v", err)
	}
	if result.Output != "true" {
		t.Errorf("expected true, got %q", result.Output)
	}
}

func TestFileBackend_Mkdir(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "subdir", "nested")

	result, err := b.Run(context.Background(), "mkdir", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("mkdir error: %v", err)
	}
	if !strings.Contains(result.Output, "created") {
		t.Errorf("expected 'created' in output, got %q", result.Output)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}

func TestFileBackend_Delete(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "todelete.txt")
	os.WriteFile(path, []byte("x"), 0644)

	result, err := b.Run(context.Background(), "delete", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if !strings.Contains(result.Output, "deleted") {
		t.Errorf("expected 'deleted' in output, got %q", result.Output)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after delete")
	}
}

func TestFileBackend_DeleteRefusesDir(t *testing.T) {
	b, dir := newTestFileBackend(t)
	subdir := filepath.Join(dir, "nodeldir")
	os.Mkdir(subdir, 0755)

	_, err := b.Run(context.Background(), "delete", map[string]string{"path": subdir})
	if err == nil {
		t.Fatal("expected error when deleting directory")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("expected directory error, got %v", err)
	}
}

func TestFileBackend_Checksum(t *testing.T) {
	b, dir := newTestFileBackend(t)
	path := filepath.Join(dir, "checksum.txt")
	os.WriteFile(path, []byte("hello world\n"), 0644)

	result, err := b.Run(context.Background(), "checksum", map[string]string{"path": path})
	if err != nil {
		t.Fatalf("checksum error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["algorithm"] != "sha256" {
		t.Errorf("expected sha256, got %v", data["algorithm"])
	}
	hash, ok := data["hash"].(string)
	if !ok || len(hash) != 64 {
		t.Errorf("expected 64-char sha256 hash, got %q", hash)
	}
}

func TestFileBackend_PathValidation(t *testing.T) {
	b, _ := newTestFileBackend(t) // only temp dir is allowed

	// Path outside allowed
	_, err := b.Run(context.Background(), "exists", map[string]string{"path": "/etc/passwd"})
	if err == nil {
		t.Fatal("expected path validation error")
	}

	// Path traversal
	_, err = b.Run(context.Background(), "exists", map[string]string{"path": "/tmp/../etc/passwd"})
	if err == nil {
		t.Fatal("expected path traversal error")
	}

	// Missing path
	_, err = b.Run(context.Background(), "read", map[string]string{})
	if err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestFileBackend_UnknownAction(t *testing.T) {
	b := &FileBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
