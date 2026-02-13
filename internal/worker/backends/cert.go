package backends

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

type CertBackend struct {
	allowedPaths []string
}

func init() {
	registerBackend("cert", &CertBackend{})
}

func (c *CertBackend) SetAllowedPaths(paths []string) {
	c.allowedPaths = paths
}

func (c *CertBackend) Actions() []string {
	return []string{"inspect", "verify", "expiry", "remote", "fingerprint"}
}

func (c *CertBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
	switch action {
	case "inspect":
		return c.inspect(params)
	case "verify":
		return c.verify(params)
	case "expiry":
		return c.expiry(params)
	case "remote":
		return c.remote(ctx, params)
	case "fingerprint":
		return c.fingerprintAction(params)
	default:
		return nil, fmt.Errorf("cert: unknown action %q", action)
	}
}

func (c *CertBackend) loadCert(path string) (*x509.Certificate, error) {
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cert: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM data found in %s", path)
	}

	// Refuse private keys
	if strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, fmt.Errorf("refusing to process private key file %s", path)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert, nil
}

func certInfo(cert *x509.Certificate) map[string]any {
	now := time.Now()
	daysUntilExpiry := int(time.Until(cert.NotAfter).Hours() / 24)
	return map[string]any{
		"subject":           cert.Subject.String(),
		"issuer":            cert.Issuer.String(),
		"not_before":        cert.NotBefore.UTC().Format(time.RFC3339),
		"not_after":         cert.NotAfter.UTC().Format(time.RFC3339),
		"dns_names":         cert.DNSNames,
		"expired":           now.After(cert.NotAfter),
		"days_until_expiry": daysUntilExpiry,
	}
}

func (c *CertBackend) inspect(params map[string]string) (*Result, error) {
	path, err := requireParam("cert", "inspect", "path", params)
	if err != nil {
		return nil, err
	}
	cert, err := c.loadCert(path)
	if err != nil {
		return nil, fmt.Errorf("cert inspect: %w", err)
	}
	return jsonResult(certInfo(cert))
}

func (c *CertBackend) verify(params map[string]string) (*Result, error) {
	path, err := requireParam("cert", "verify", "path", params)
	if err != nil {
		return nil, err
	}
	cert, err := c.loadCert(path)
	if err != nil {
		return nil, fmt.Errorf("cert verify: %w", err)
	}

	opts := x509.VerifyOptions{}

	if caPath := params["ca_path"]; caPath != "" {
		if err := ValidatePath(caPath, c.allowedPaths); err != nil {
			return nil, err
		}
		caData, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("cert verify: reading CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("cert verify: no valid CA certs found in %s", caPath)
		}
		opts.Roots = pool
	}

	chains, err := cert.Verify(opts)
	valid := err == nil

	out := map[string]any{
		"valid":       valid,
		"chain_count": len(chains),
	}
	if err != nil {
		out["error"] = err.Error()
	}
	return jsonResult(out)
}

func (c *CertBackend) expiry(params map[string]string) (*Result, error) {
	path, err := requireParam("cert", "expiry", "path", params)
	if err != nil {
		return nil, err
	}
	cert, err := c.loadCert(path)
	if err != nil {
		return nil, fmt.Errorf("cert expiry: %w", err)
	}

	now := time.Now()
	out := map[string]any{
		"path":           path,
		"not_after":      cert.NotAfter.UTC().Format(time.RFC3339),
		"days_remaining": int(time.Until(cert.NotAfter).Hours() / 24),
		"expired":        now.After(cert.NotAfter),
	}
	return jsonResult(out)
}

func (c *CertBackend) remote(ctx context.Context, params map[string]string) (*Result, error) {
	host, err := requireParam("cert", "remote", "host", params)
	if err != nil {
		return nil, err
	}
	port := params["port"]
	if port == "" {
		port = "443"
	}

	addr := net.JoinHostPort(host, port)
	dialer := &tls.Dialer{
		Config: &tls.Config{
			InsecureSkipVerify: true, // We're inspecting, not trusting
		},
	}

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cert remote: connecting to %s: %w", addr, err)
	}
	defer conn.Close()

	tlsConn := conn.(*tls.Conn)
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("cert remote: no certificates from %s", addr)
	}

	return jsonResult(certInfo(certs[0]))
}

func (c *CertBackend) fingerprintAction(params map[string]string) (*Result, error) {
	path, err := requireParam("cert", "fingerprint", "path", params)
	if err != nil {
		return nil, err
	}
	if err := ValidatePath(path, c.allowedPaths); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cert fingerprint: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("cert fingerprint: no PEM data in %s", path)
	}
	if strings.Contains(block.Type, "PRIVATE KEY") {
		return nil, fmt.Errorf("cert fingerprint: refusing to process private key")
	}

	algorithm := params["algorithm"]
	if algorithm == "" {
		algorithm = "sha256"
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("cert fingerprint: %w", err)
	}

	fp := certFingerprint(cert.Raw, algorithm)
	if fp == "" {
		return nil, fmt.Errorf("cert fingerprint: unsupported algorithm %q (sha256, sha512, md5)", algorithm)
	}

	out := map[string]any{
		"path":        path,
		"algorithm":   algorithm,
		"fingerprint": fp,
	}
	return jsonResult(out)
}

func certFingerprint(raw []byte, algorithm string) string {
	switch algorithm {
	case "sha256":
		h := sha256.Sum256(raw)
		return fmt.Sprintf("%x", h)
	case "sha512":
		h := sha512.Sum512(raw)
		return fmt.Sprintf("%x", h)
	case "md5":
		h := md5.Sum(raw)
		return fmt.Sprintf("%x", h)
	default:
		return ""
	}
}
