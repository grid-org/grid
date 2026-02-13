package backends

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type NetBackend struct{}

func init() {
	registerBackend("net", &NetBackend{})
}

func (n *NetBackend) Actions() []string {
	return []string{"ping", "resolve", "port", "interfaces", "traceroute"}
}

func (n *NetBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "ping":
		return n.ping(ctx, params)
	case "resolve":
		return n.resolve(ctx, params)
	case "port":
		return n.port(ctx, params)
	case "interfaces":
		return n.interfaces()
	case "traceroute":
		return n.traceroute(ctx, params)
	default:
		return nil, fmt.Errorf("net: unknown action %q", action)
	}
}

func (n *NetBackend) ping(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("net", "ping", "host", params)
	if err != nil {
		return nil, err
	}

	count := 3
	if c := params["count"]; c != "" {
		count, err = strconv.Atoi(c)
		if err != nil || count < 1 {
			return nil, fmt.Errorf("net ping: invalid count %q", c)
		}
		if count > 10 {
			count = 10
		}
	}

	out, err := exec.CommandContext(ctx, "ping", "-c", strconv.Itoa(count), "-W", "5", host).CombinedOutput()
	if err != nil {
		// ping returns exit code 1 on packet loss — still useful output
		if len(out) > 0 {
			return &Result{Output: truncateOutput(string(out))}, nil
		}
		return nil, fmt.Errorf("net ping: %w", err)
	}
	return &Result{Output: truncateOutput(string(out))}, nil
}

func (n *NetBackend) resolve(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("net", "resolve", "host", params)
	if err != nil {
		return nil, err
	}

	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("net resolve: %w", err)
	}

	out := map[string]any{
		"host":      host,
		"addresses": addrs,
	}
	return jsonResult(out)
}

func (n *NetBackend) port(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("net", "port", "host", params)
	if err != nil {
		return nil, err
	}
	portStr, err := requireParam("net", "port", "port", params)
	if err != nil {
		return nil, err
	}
	portNum, err := strconv.Atoi(portStr)
	if err != nil || portNum < 1 || portNum > 65535 {
		return nil, fmt.Errorf("net port: invalid port %q", portStr)
	}

	timeout := 5 * time.Second
	if t := params["timeout"]; t != "" {
		timeout, err = time.ParseDuration(t)
		if err != nil {
			return nil, fmt.Errorf("net port: invalid timeout %q: %w", t, err)
		}
	}

	addr := net.JoinHostPort(host, portStr)
	start := time.Now()

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(dialCtx, "tcp", addr)
	latency := time.Since(start)

	open := err == nil
	if open {
		conn.Close()
	}

	out := map[string]any{
		"host":       host,
		"port":       portNum,
		"open":       open,
		"latency_ms": latency.Milliseconds(),
	}
	return jsonResult(out)
}

func (n *NetBackend) interfaces() (*Result, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("net interfaces: %w", err)
	}

	var result []map[string]any
	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		addrStrs := make([]string, 0, len(addrs))
		for _, a := range addrs {
			addrStrs = append(addrStrs, a.String())
		}

		result = append(result, map[string]any{
			"name":      iface.Name,
			"mac":       iface.HardwareAddr.String(),
			"addresses": addrStrs,
			"flags":     iface.Flags.String(),
		})
	}

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("net interfaces: %w", err)
	}
	return &Result{Output: truncateOutput(string(data))}, nil
}

func (n *NetBackend) traceroute(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("net", "traceroute", "host", params)
	if err != nil {
		return nil, err
	}

	maxHops := 15
	if m := params["max_hops"]; m != "" {
		maxHops, err = strconv.Atoi(m)
		if err != nil || maxHops < 1 {
			return nil, fmt.Errorf("net traceroute: invalid max_hops %q", m)
		}
		if maxHops > 30 {
			maxHops = 30
		}
	}

	// Try traceroute first, fall back to tracepath
	args := []string{"-m", strconv.Itoa(maxHops), "-n", host}
	out, err := exec.CommandContext(ctx, "traceroute", args...).CombinedOutput()
	if err != nil {
		// Fallback to tracepath (commonly available on Linux without privileges)
		args = []string{"-m", strconv.Itoa(maxHops), "-n", host}
		out, err = exec.CommandContext(ctx, "tracepath", args...).CombinedOutput()
		if err != nil {
			if len(out) > 0 {
				return &Result{Output: truncateOutput(string(out))}, nil
			}
			return nil, fmt.Errorf("net traceroute: %w (traceroute and tracepath both failed)", err)
		}
	}

	return &Result{Output: truncateOutput(strings.TrimSpace(string(out)))}, nil
}
