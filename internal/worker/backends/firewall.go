package backends

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

type FirewallBackend struct{}

func init() {
	registerBackend("firewall", &FirewallBackend{})
}

func (f *FirewallBackend) Actions() []string {
	return []string{"list", "allow", "deny", "remove", "status"}
}

func (f *FirewallBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "list":
		return f.list(ctx, params)
	case "allow":
		return f.allow(ctx, params)
	case "deny":
		return f.deny(ctx, params)
	case "remove":
		return f.remove(ctx, params)
	case "status":
		return f.status(ctx)
	default:
		return nil, fmt.Errorf("firewall: unknown action %q", action)
	}
}

// detectFirewall checks which firewall tool is available.
func detectFirewall() string {
	if _, err := exec.LookPath("ufw"); err == nil {
		return "ufw"
	}
	if _, err := exec.LookPath("nft"); err == nil {
		return "nft"
	}
	if _, err := exec.LookPath("iptables"); err == nil {
		return "iptables"
	}
	return ""
}

func validatePort(portStr string) (int, error) {
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("firewall: invalid port %q (1-65535)", portStr)
	}
	return port, nil
}

func validateProto(proto string) error {
	if proto != "tcp" && proto != "udp" {
		return fmt.Errorf("firewall: invalid protocol %q (tcp/udp only)", proto)
	}
	return nil
}

func validateSource(source string) error {
	if source == "" {
		return nil
	}
	// Accept IP or CIDR
	if net.ParseIP(source) != nil {
		return nil
	}
	if _, _, err := net.ParseCIDR(source); err == nil {
		return nil
	}
	return fmt.Errorf("firewall: invalid source %q (IP or CIDR required)", source)
}

func (f *FirewallBackend) list(ctx context.Context, params map[string]string) (*Result, error) {
	fw := detectFirewall()
	switch fw {
	case "ufw":
		out, err := exec.CommandContext(ctx, "ufw", "status", "numbered").CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	case "nft":
		args := []string{"list", "ruleset"}
		out, err := exec.CommandContext(ctx, "nft", args...).CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	case "iptables":
		chain := params["chain"]
		args := []string{"-L", "-n", "--line-numbers"}
		if chain != "" {
			args = append(args, chain)
		}
		out, err := exec.CommandContext(ctx, "iptables", args...).CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	default:
		return nil, fmt.Errorf("firewall: no supported firewall found (ufw, nft, or iptables)")
	}
}

func (f *FirewallBackend) allow(ctx context.Context, params map[string]string) (*Result, error) {
	portStr, err := requireParam("firewall", "allow", "port", params)
	if err != nil {
		return nil, err
	}
	port, err := validatePort(portStr)
	if err != nil {
		return nil, err
	}

	proto := params["proto"]
	if proto == "" {
		proto = "tcp"
	}
	if err := validateProto(proto); err != nil {
		return nil, err
	}

	source := params["source"]
	if err := validateSource(source); err != nil {
		return nil, err
	}

	fw := detectFirewall()
	switch fw {
	case "ufw":
		rule := fmt.Sprintf("%d/%s", port, proto)
		args := []string{"allow"}
		if source != "" {
			args = append(args, "from", source, "to", "any", "port", strconv.Itoa(port), "proto", proto)
		} else {
			args = append(args, rule)
		}
		out, err := exec.CommandContext(ctx, "ufw", args...).CombinedOutput()
		return &Result{Output: strings.TrimSpace(string(out))}, err
	case "iptables":
		args := []string{"-A", "INPUT", "-p", proto, "--dport", strconv.Itoa(port), "-j", "ACCEPT"}
		if source != "" {
			args = []string{"-A", "INPUT", "-s", source, "-p", proto, "--dport", strconv.Itoa(port), "-j", "ACCEPT"}
		}
		out, err := exec.CommandContext(ctx, "iptables", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("firewall allow: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return &Result{Output: fmt.Sprintf("allowed %s/%d", proto, port)}, nil
	default:
		return nil, fmt.Errorf("firewall: no supported firewall found")
	}
}

func (f *FirewallBackend) deny(ctx context.Context, params map[string]string) (*Result, error) {
	portStr, err := requireParam("firewall", "deny", "port", params)
	if err != nil {
		return nil, err
	}
	port, err := validatePort(portStr)
	if err != nil {
		return nil, err
	}

	proto := params["proto"]
	if proto == "" {
		proto = "tcp"
	}
	if err := validateProto(proto); err != nil {
		return nil, err
	}

	source := params["source"]
	if err := validateSource(source); err != nil {
		return nil, err
	}

	fw := detectFirewall()
	switch fw {
	case "ufw":
		rule := fmt.Sprintf("%d/%s", port, proto)
		args := []string{"deny"}
		if source != "" {
			args = append(args, "from", source, "to", "any", "port", strconv.Itoa(port), "proto", proto)
		} else {
			args = append(args, rule)
		}
		out, err := exec.CommandContext(ctx, "ufw", args...).CombinedOutput()
		return &Result{Output: strings.TrimSpace(string(out))}, err
	case "iptables":
		args := []string{"-A", "INPUT", "-p", proto, "--dport", strconv.Itoa(port), "-j", "DROP"}
		if source != "" {
			args = []string{"-A", "INPUT", "-s", source, "-p", proto, "--dport", strconv.Itoa(port), "-j", "DROP"}
		}
		out, err := exec.CommandContext(ctx, "iptables", args...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("firewall deny: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return &Result{Output: fmt.Sprintf("denied %s/%d", proto, port)}, nil
	default:
		return nil, fmt.Errorf("firewall: no supported firewall found")
	}
}

func (f *FirewallBackend) remove(ctx context.Context, params map[string]string) (*Result, error) {
	portStr, err := requireParam("firewall", "remove", "port", params)
	if err != nil {
		return nil, err
	}
	port, err := validatePort(portStr)
	if err != nil {
		return nil, err
	}

	proto := params["proto"]
	if proto == "" {
		proto = "tcp"
	}
	if err := validateProto(proto); err != nil {
		return nil, err
	}

	fw := detectFirewall()
	switch fw {
	case "ufw":
		rule := fmt.Sprintf("%d/%s", port, proto)
		out, err := exec.CommandContext(ctx, "ufw", "delete", "allow", rule).CombinedOutput()
		return &Result{Output: strings.TrimSpace(string(out))}, err
	case "iptables":
		// Try to delete both ACCEPT and DROP rules
		exec.CommandContext(ctx, "iptables", "-D", "INPUT", "-p", proto, "--dport", strconv.Itoa(port), "-j", "ACCEPT").CombinedOutput()
		exec.CommandContext(ctx, "iptables", "-D", "INPUT", "-p", proto, "--dport", strconv.Itoa(port), "-j", "DROP").CombinedOutput()
		return &Result{Output: fmt.Sprintf("removed rule for %s/%d", proto, port)}, nil
	default:
		return nil, fmt.Errorf("firewall: no supported firewall found")
	}
}

func (f *FirewallBackend) status(ctx context.Context) (*Result, error) {
	fw := detectFirewall()
	switch fw {
	case "ufw":
		out, err := exec.CommandContext(ctx, "ufw", "status", "verbose").CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	case "iptables":
		out, err := exec.CommandContext(ctx, "iptables", "-L", "-n", "-v").CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	case "nft":
		out, err := exec.CommandContext(ctx, "nft", "list", "ruleset").CombinedOutput()
		return &Result{Output: truncateOutput(string(out))}, err
	default:
		return nil, fmt.Errorf("firewall: no supported firewall found (ufw, nft, or iptables)")
	}
}
