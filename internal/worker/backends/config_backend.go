package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

type ConfigBackend struct {
	allowedPaths []string
}

func init() {
	registerBackend("config", &ConfigBackend{})
}

func (c *ConfigBackend) SetAllowedPaths(paths []string) {
	c.allowedPaths = paths
}

func (c *ConfigBackend) Actions() []string {
	return []string{"get", "set", "validate", "diff"}
}

func (c *ConfigBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "get":
		return c.get(params)
	case "set":
		return c.setKey(params)
	case "validate":
		return c.validate(params)
	case "diff":
		return c.diff(params)
	default:
		return nil, fmt.Errorf("config: unknown action %q", action)
	}
}

func (c *ConfigBackend) detectFormat(path, hint string) string {
	if hint != "" {
		return strings.ToLower(hint)
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return "yaml"
	case ".json":
		return "json"
	case ".ini", ".conf", ".cfg":
		return "ini"
	default:
		return "yaml" // default
	}
}

// parseFile reads and parses a config file into a generic map.
// Supports yaml and json. INI support is basic key=value per section.
func (c *ConfigBackend) parseFile(path, format string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return c.parseData(data, format)
}

func (c *ConfigBackend) parseData(data []byte, format string) (map[string]any, error) {
	var result map[string]any
	switch format {
	case "yaml":
		if err := yaml.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("parsing yaml: %w", err)
		}
	case "json":
		if err := json.Unmarshal(data, &result); err != nil {
			return nil, fmt.Errorf("parsing json: %w", err)
		}
	case "ini":
		result = parseINI(string(data))
	default:
		return nil, fmt.Errorf("unsupported format %q (yaml, json, ini)", format)
	}
	return result, nil
}

// parseINI does basic INI parsing: [section] headers and key=value pairs.
func parseINI(content string) map[string]any {
	result := make(map[string]any)
	section := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			if _, ok := result[section]; !ok {
				result[section] = make(map[string]any)
			}
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if section != "" {
				if m, ok := result[section].(map[string]any); ok {
					m[k] = v
				}
			} else {
				result[k] = v
			}
		}
	}
	return result
}

func marshalData(data map[string]any, format string) ([]byte, error) {
	switch format {
	case "yaml":
		return yaml.Marshal(data)
	case "json":
		return json.MarshalIndent(data, "", "  ")
	case "ini":
		return marshalINI(data), nil
	default:
		return nil, fmt.Errorf("unsupported format %q", format)
	}
}

func marshalINI(data map[string]any) []byte {
	var sb strings.Builder
	// Write top-level non-section keys first
	for k, v := range data {
		if _, isMap := v.(map[string]any); !isMap {
			sb.WriteString(fmt.Sprintf("%s = %v\n", k, v))
		}
	}
	// Write sections
	for k, v := range data {
		if section, isMap := v.(map[string]any); isMap {
			sb.WriteString(fmt.Sprintf("\n[%s]\n", k))
			for sk, sv := range section {
				sb.WriteString(fmt.Sprintf("%s = %v\n", sk, sv))
			}
		}
	}
	return []byte(sb.String())
}

// getNestedKey traverses a map by dot-notation key path.
func getNestedKey(data map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	var current any = data
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = m[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// setNestedKey sets a value in a map by dot-notation key path, creating intermediate maps.
func setNestedKey(data map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	current := data
	for i, part := range parts {
		if i == len(parts)-1 {
			current[part] = value
			return
		}
		next, ok := current[part]
		if !ok {
			next = make(map[string]any)
			current[part] = next
		}
		if m, ok := next.(map[string]any); ok {
			current = m
		} else {
			// Overwrite non-map intermediate
			m := make(map[string]any)
			current[part] = m
			current = m
		}
	}
}

func (c *ConfigBackend) get(params map[string]string) (*Result, error) {
	path, err := requireParam("config", "get", "path", params)
	if err != nil {
		return nil, err
	}
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}
	key, err := requireParam("config", "get", "key", params)
	if err != nil {
		return nil, err
	}

	format := c.detectFormat(path, params["format"])
	data, err := c.parseFile(path, format)
	if err != nil {
		return nil, fmt.Errorf("config get: %w", err)
	}

	val, ok := getNestedKey(data, key)
	if !ok {
		return nil, fmt.Errorf("config get: key %q not found", key)
	}

	switch v := val.(type) {
	case string:
		return &Result{Output: v}, nil
	default:
		b, _ := json.Marshal(v)
		return &Result{Output: string(b)}, nil
	}
}

func (c *ConfigBackend) setKey(params map[string]string) (*Result, error) {
	path, err := requireParam("config", "set", "path", params)
	if err != nil {
		return nil, err
	}
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}
	key, err := requireParam("config", "set", "key", params)
	if err != nil {
		return nil, err
	}
	value, err := requireParam("config", "set", "value", params)
	if err != nil {
		return nil, err
	}

	format := c.detectFormat(path, params["format"])

	// Read existing file
	data := make(map[string]any)
	if _, statErr := os.Stat(path); statErr == nil {
		data, err = c.parseFile(path, format)
		if err != nil {
			return nil, fmt.Errorf("config set: %w", err)
		}
	}

	// Create backup
	if _, statErr := os.Stat(path); statErr == nil {
		existing, _ := os.ReadFile(path)
		os.WriteFile(path+".bak", existing, 0644)
	}

	setNestedKey(data, key, value)

	out, err := marshalData(data, format)
	if err != nil {
		return nil, fmt.Errorf("config set: marshaling: %w", err)
	}

	if err := os.WriteFile(path, out, 0644); err != nil {
		return nil, fmt.Errorf("config set: writing: %w", err)
	}

	return &Result{Output: fmt.Sprintf("set %s=%s in %s", key, value, path)}, nil
}

func (c *ConfigBackend) validate(params map[string]string) (*Result, error) {
	path, err := requireParam("config", "validate", "path", params)
	if err != nil {
		return nil, err
	}
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}
	format, err := requireParam("config", "validate", "format", params)
	if err != nil {
		return nil, err
	}

	_, parseErr := c.parseFile(path, strings.ToLower(format))

	out := map[string]any{
		"path":   path,
		"format": format,
		"valid":  parseErr == nil,
	}
	if parseErr != nil {
		out["error"] = parseErr.Error()
	}
	return jsonResult(out)
}

func (c *ConfigBackend) diff(params map[string]string) (*Result, error) {
	path, err := requireParam("config", "diff", "path", params)
	if err != nil {
		return nil, err
	}
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}
	content, err := requireParam("config", "diff", "content", params)
	if err != nil {
		return nil, err
	}

	existing, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config diff: %w", err)
	}

	// Simple line-by-line diff
	oldLines := strings.Split(string(existing), "\n")
	newLines := strings.Split(content, "\n")

	var diff strings.Builder
	diff.WriteString(fmt.Sprintf("--- %s\n+++ proposed\n", path))

	maxLen := len(oldLines)
	if len(newLines) > maxLen {
		maxLen = len(newLines)
	}

	for i := 0; i < maxLen; i++ {
		old := ""
		if i < len(oldLines) {
			old = oldLines[i]
		}
		new := ""
		if i < len(newLines) {
			new = newLines[i]
		}
		if old != new {
			if i < len(oldLines) {
				diff.WriteString(fmt.Sprintf("-%s\n", old))
			}
			if i < len(newLines) {
				diff.WriteString(fmt.Sprintf("+%s\n", new))
			}
		} else {
			diff.WriteString(fmt.Sprintf(" %s\n", old))
		}
	}

	return &Result{Output: truncateOutput(diff.String())}, nil
}
