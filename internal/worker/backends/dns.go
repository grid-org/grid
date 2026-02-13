package backends

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
)

var allowedDNSTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "MX": true,
	"TXT": true, "NS": true, "SRV": true, "PTR": true,
}

type DNSBackend struct{}

func init() {
	registerBackend("dns", &DNSBackend{})
}

func (d *DNSBackend) Actions() []string {
	return []string{"lookup", "reverse", "resolvers", "check"}
}

func (d *DNSBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "lookup":
		return d.lookup(ctx, params)
	case "reverse":
		return d.reverse(ctx, params)
	case "resolvers":
		return d.resolverList()
	case "check":
		return d.check(ctx, params)
	default:
		return nil, fmt.Errorf("dns: unknown action %q", action)
	}
}

func (d *DNSBackend) lookup(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("dns", "lookup", "name", params)
	if err != nil {
		return nil, err
	}

	qtype := params["type"]
	if qtype == "" {
		qtype = "A"
	}
	qtype = strings.ToUpper(qtype)
	if !allowedDNSTypes[qtype] {
		return nil, fmt.Errorf("dns lookup: unsupported record type %q (allowed: A, AAAA, CNAME, MX, TXT, NS, SRV, PTR)", qtype)
	}

	r := net.DefaultResolver
	var records []string

	switch qtype {
	case "A", "AAAA":
		addrs, err := r.LookupHost(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		for _, a := range addrs {
			ip := net.ParseIP(a)
			if ip == nil {
				continue
			}
			if qtype == "A" && ip.To4() != nil {
				records = append(records, a)
			} else if qtype == "AAAA" && ip.To4() == nil {
				records = append(records, a)
			}
		}
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		records = append(records, cname)
	case "MX":
		mxs, err := r.LookupMX(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		for _, mx := range mxs {
			records = append(records, fmt.Sprintf("%d %s", mx.Pref, mx.Host))
		}
	case "TXT":
		txts, err := r.LookupTXT(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		records = txts
	case "NS":
		nss, err := r.LookupNS(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		for _, ns := range nss {
			records = append(records, ns.Host)
		}
	case "SRV":
		_, srvs, err := r.LookupSRV(ctx, "", "", name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		for _, srv := range srvs {
			records = append(records, fmt.Sprintf("%d %d %d %s", srv.Priority, srv.Weight, srv.Port, srv.Target))
		}
	case "PTR":
		names, err := r.LookupAddr(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("dns lookup: %w", err)
		}
		records = names
	}

	out := map[string]any{
		"name":    name,
		"type":    qtype,
		"records": records,
	}
	return jsonResult(out)
}

func (d *DNSBackend) reverse(ctx context.Context, params map[string]string) (*Result, error) {
	ip, err := requireParam("dns", "reverse", "ip", params)
	if err != nil {
		return nil, err
	}

	names, err := net.DefaultResolver.LookupAddr(ctx, ip)
	if err != nil {
		return nil, fmt.Errorf("dns reverse: %w", err)
	}

	out := map[string]any{
		"ip":    ip,
		"names": names,
	}
	return jsonResult(out)
}

func (d *DNSBackend) resolverList() (*Result, error) {
	// Read /etc/resolv.conf for configured resolvers
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil, fmt.Errorf("dns resolvers: %w", err)
	}

	var resolvers []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver ") {
			resolvers = append(resolvers, strings.TrimPrefix(line, "nameserver "))
		}
	}

	out := map[string]any{
		"resolvers": resolvers,
	}
	return jsonResult(out)
}

func (d *DNSBackend) check(ctx context.Context, params map[string]string) (*Result, error) {
	name, err := requireParam("dns", "check", "name", params)
	if err != nil {
		return nil, err
	}
	expected, err := requireParam("dns", "check", "expected", params)
	if err != nil {
		return nil, err
	}

	addrs, err := net.DefaultResolver.LookupHost(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("dns check: %w", err)
	}

	matches := false
	for _, a := range addrs {
		if a == expected {
			matches = true
			break
		}
	}

	out := map[string]any{
		"name":     name,
		"expected": expected,
		"actual":   addrs,
		"matches":  matches,
	}
	return jsonResult(out)
}
