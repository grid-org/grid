package backends

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

type HealthBackend struct{}

func init() {
	registerBackend("health", &HealthBackend{})
}

func (h *HealthBackend) Actions() []string {
	return []string{"http", "tcp", "disk_space", "memory", "load", "process"}
}

func (h *HealthBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "http":
		return h.httpCheck(ctx, params)
	case "tcp":
		return h.tcpCheck(ctx, params)
	case "disk_space":
		return h.diskSpace(ctx, params)
	case "memory":
		return h.memoryCheck(ctx, params)
	case "load":
		return h.loadCheck(ctx, params)
	case "process":
		return h.processCheck(ctx, params)
	default:
		return nil, fmt.Errorf("health: unknown action %q", action)
	}
}

func (h *HealthBackend) httpCheck(ctx context.Context, params map[string]string) (*Result, error) {
	rawURL, err := requireParam("health", "http", "url", params)
	if err != nil {
		return nil, err
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("health http: invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("health http: only http/https URLs allowed, got %q", parsed.Scheme)
	}

	method := "GET"
	if m := params["method"]; m != "" {
		m = strings.ToUpper(m)
		if m != "GET" && m != "HEAD" {
			return nil, fmt.Errorf("health http: only GET and HEAD methods allowed, got %q", m)
		}
		method = m
	}

	expectedStatus := 200
	if e := params["expected_status"]; e != "" {
		fmt.Sscanf(e, "%d", &expectedStatus)
	}

	timeout := 10 * time.Second
	if t := params["timeout"]; t != "" {
		timeout, err = time.ParseDuration(t)
		if err != nil {
			return nil, fmt.Errorf("health http: invalid timeout %q: %w", t, err)
		}
	}

	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("health http: %w", err)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latency := time.Since(start)

	if err != nil {
		out := map[string]any{
			"url":       rawURL,
			"error":     err.Error(),
			"matches":   false,
			"latency_ms": latency.Milliseconds(),
		}
		return jsonResult(out)
	}
	defer resp.Body.Close()

	out := map[string]any{
		"url":             rawURL,
		"status_code":     resp.StatusCode,
		"latency_ms":      latency.Milliseconds(),
		"content_length":  resp.ContentLength,
		"matches":         resp.StatusCode == expectedStatus,
	}
	return jsonResult(out)
}

func (h *HealthBackend) tcpCheck(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("health", "tcp", "host", params)
	if err != nil {
		return nil, err
	}
	port, err := requireParam("health", "tcp", "port", params)
	if err != nil {
		return nil, err
	}

	timeout := 5 * time.Second
	if t := params["timeout"]; t != "" {
		timeout, err = time.ParseDuration(t)
		if err != nil {
			return nil, fmt.Errorf("health tcp: invalid timeout %q: %w", t, err)
		}
	}

	addr := net.JoinHostPort(host, port)
	start := time.Now()

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, dialErr := d.DialContext(dialCtx, "tcp", addr)
	latency := time.Since(start)

	reachable := dialErr == nil
	if reachable {
		conn.Close()
	}

	out := map[string]any{
		"host":       host,
		"port":       port,
		"reachable":  reachable,
		"latency_ms": latency.Milliseconds(),
	}
	return jsonResult(out)
}

func (h *HealthBackend) diskSpace(ctx context.Context, params map[string]string) (*Result, error) {
	mount := params["mount"]
	if mount == "" {
		mount = "/"
	}

	threshold := 90.0
	if t := params["threshold_percent"]; t != "" {
		fmt.Sscanf(t, "%f", &threshold)
	}

	usage, err := disk.UsageWithContext(ctx, mount)
	if err != nil {
		return nil, fmt.Errorf("health disk_space: %w", err)
	}

	out := map[string]any{
		"mount":         mount,
		"usage_percent": usage.UsedPercent,
		"threshold":     threshold,
		"healthy":       usage.UsedPercent < threshold,
	}
	return jsonResult(out)
}

func (h *HealthBackend) memoryCheck(ctx context.Context, params map[string]string) (*Result, error) {
	threshold := 90.0
	if t := params["threshold_percent"]; t != "" {
		fmt.Sscanf(t, "%f", &threshold)
	}

	v, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("health memory: %w", err)
	}

	out := map[string]any{
		"usage_percent": v.UsedPercent,
		"threshold":     threshold,
		"healthy":       v.UsedPercent < threshold,
	}
	return jsonResult(out)
}

func (h *HealthBackend) loadCheck(ctx context.Context, params map[string]string) (*Result, error) {
	avg, err := load.AvgWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("health load: %w", err)
	}

	threshold := 0.0
	hasThreshold := false
	if t := params["threshold"]; t != "" {
		fmt.Sscanf(t, "%f", &threshold)
		hasThreshold = true
	}

	healthy := true
	if hasThreshold {
		healthy = avg.Load1 < threshold
	}

	out := map[string]any{
		"load1":   avg.Load1,
		"load5":   avg.Load5,
		"load15":  avg.Load15,
		"healthy": healthy,
	}
	if hasThreshold {
		out["threshold"] = threshold
	}
	return jsonResult(out)
}

func (h *HealthBackend) processCheck(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("health", "process", "name", params)
	if err != nil {
		return nil, err
	}

	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("health process: %w", err)
	}

	nameLower := strings.ToLower(name)
	var matchPID int32
	matchCount := 0
	for _, p := range procs {
		pName, _ := p.NameWithContext(ctx)
		if strings.ToLower(pName) == nameLower {
			matchCount++
			if matchPID == 0 {
				matchPID = p.Pid
			}
		}
	}

	out := map[string]any{
		"name":    name,
		"running": matchCount > 0,
		"pid":     matchPID,
		"count":   matchCount,
	}
	return jsonResult(out)
}
