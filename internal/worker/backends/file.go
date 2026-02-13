package backends

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type FileBackend struct {
	allowedPaths []string
}

func init() {
	registerBackend("file", &FileBackend{})
}

func (f *FileBackend) SetAllowedPaths(paths []string) {
	f.allowedPaths = paths
}

func (f *FileBackend) Actions() []string {
	return []string{"read", "write", "stat", "exists", "mkdir", "delete", "checksum"}
}

func (f *FileBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "read":
		return f.read(params)
	case "write":
		return f.write(params)
	case "stat":
		return f.stat(params)
	case "exists":
		return f.exists(params)
	case "mkdir":
		return f.mkdirAction(params)
	case "delete":
		return f.delete(params)
	case "checksum":
		return f.checksum(params)
	default:
		return nil, fmt.Errorf("file: unknown action %q", action)
	}
}

func (f *FileBackend) validatePath(path string) error {
	return ValidatePath(path, f.allowedPaths)
}

func (f *FileBackend) read(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "read", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	lines := 100
	if l := params["lines"]; l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("file read: invalid lines %q", l)
		}
		if n > 1000 {
			n = 1000
		}
		lines = n
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("file read: %w", err)
	}
	defer file.Close()

	// Read all lines, keep last N (tail behavior)
	var allLines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("file read: %w", err)
	}

	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}
	output := strings.Join(allLines[start:], "\n")

	return &Result{Output: truncateOutput(output)}, nil
}

func (f *FileBackend) write(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "write", "path", params)
	if err != nil {
		return nil, err
	}
	content, err := requireParam("file", "write", "content", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	mode := os.FileMode(0644)
	if m := params["mode"]; m != "" {
		parsed, err := strconv.ParseUint(m, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("file write: invalid mode %q: %w", m, err)
		}
		mode = os.FileMode(parsed)
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		return nil, fmt.Errorf("file write: %w", err)
	}

	return &Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(content), path)}, nil
}

func (f *FileBackend) stat(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "stat", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("file stat: %w", err)
	}

	out := map[string]any{
		"path":     path,
		"size":     info.Size(),
		"mode":     fmt.Sprintf("%04o", info.Mode().Perm()),
		"mod_time": info.ModTime().UTC().Format(time.RFC3339),
		"is_dir":   info.IsDir(),
	}
	return jsonResult(out)
}

func (f *FileBackend) exists(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "exists", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return &Result{Output: "false"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("file exists: %w", err)
	}
	return &Result{Output: "true"}, nil
}

func (f *FileBackend) mkdirAction(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "mkdir", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	mode := os.FileMode(0755)
	if m := params["mode"]; m != "" {
		parsed, err := strconv.ParseUint(m, 8, 32)
		if err != nil {
			return nil, fmt.Errorf("file mkdir: invalid mode %q: %w", m, err)
		}
		mode = os.FileMode(parsed)
	}

	if err := os.MkdirAll(path, mode); err != nil {
		return nil, fmt.Errorf("file mkdir: %w", err)
	}

	return &Result{Output: fmt.Sprintf("created %s", path)}, nil
}

func (f *FileBackend) delete(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "delete", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	// Refuse symlinks — resolve first to prevent deletion of symlink targets
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("file delete: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("file delete: refusing to delete symlink %s", path)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("file delete: refusing to delete directory %s (single files only)", path)
	}

	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("file delete: %w", err)
	}

	return &Result{Output: fmt.Sprintf("deleted %s", path)}, nil
}

func (f *FileBackend) checksum(params map[string]string) (*Result, error) {
	path, err := requireParam("file", "checksum", "path", params)
	if err != nil {
		return nil, err
	}
	if err := f.validatePath(path); err != nil {
		return nil, err
	}

	algorithm := params["algorithm"]
	if algorithm == "" {
		algorithm = "sha256"
	}

	var h hash.Hash
	switch algorithm {
	case "sha256":
		h = sha256.New()
	case "sha512":
		h = sha512.New()
	case "md5":
		h = md5.New()
	default:
		return nil, fmt.Errorf("file checksum: unsupported algorithm %q (sha256, sha512, md5)", algorithm)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("file checksum: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(h, file); err != nil {
		return nil, fmt.Errorf("file checksum: %w", err)
	}

	out := map[string]any{
		"path":      path,
		"algorithm": algorithm,
		"hash":      fmt.Sprintf("%x", h.Sum(nil)),
	}
	return jsonResult(out)
}
