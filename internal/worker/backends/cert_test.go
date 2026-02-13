package backends

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func generateTestCert(t *testing.T, dir string) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"test.example.com", "localhost"},
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	certPath := filepath.Join(dir, "test.crt")
	f, err := os.Create(certPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: certBytes})
	return certPath
}

func TestCertBackend_Actions(t *testing.T) {
	b := &CertBackend{}
	want := map[string]bool{"inspect": true, "verify": true, "expiry": true, "remote": true, "fingerprint": true}
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

func TestCertBackend_Inspect(t *testing.T) {
	dir := t.TempDir()
	certPath := generateTestCert(t, dir)
	b := &CertBackend{allowedPaths: []string{dir}}

	result, err := b.Run(context.Background(), "inspect", map[string]string{"path": certPath})
	if err != nil {
		t.Fatalf("inspect error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	for _, key := range []string{"subject", "issuer", "not_before", "not_after", "expired", "days_until_expiry"} {
		if _, ok := data[key]; !ok {
			t.Errorf("missing key %q", key)
		}
	}

	if data["expired"].(bool) {
		t.Error("cert should not be expired")
	}
}

func TestCertBackend_Expiry(t *testing.T) {
	dir := t.TempDir()
	certPath := generateTestCert(t, dir)
	b := &CertBackend{allowedPaths: []string{dir}}

	result, err := b.Run(context.Background(), "expiry", map[string]string{"path": certPath})
	if err != nil {
		t.Fatalf("expiry error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["expired"].(bool) {
		t.Error("cert should not be expired")
	}
	if days, ok := data["days_remaining"].(float64); !ok || days < 0 {
		t.Errorf("expected days_remaining >= 0, got %v", data["days_remaining"])
	}
}

func TestCertBackend_Fingerprint(t *testing.T) {
	dir := t.TempDir()
	certPath := generateTestCert(t, dir)
	b := &CertBackend{allowedPaths: []string{dir}}

	result, err := b.Run(context.Background(), "fingerprint", map[string]string{"path": certPath})
	if err != nil {
		t.Fatalf("fingerprint error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	fp, ok := data["fingerprint"].(string)
	if !ok || len(fp) != 64 {
		t.Errorf("expected 64-char sha256 fingerprint, got %q", fp)
	}
}

func TestCertBackend_RefusesPrivateKey(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "test.key")

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyBytes, _ := x509.MarshalECPrivateKey(key)
	f, _ := os.Create(keyPath)
	pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})
	f.Close()

	b := &CertBackend{allowedPaths: []string{dir}}
	_, err := b.Run(context.Background(), "inspect", map[string]string{"path": keyPath})
	if err == nil {
		t.Fatal("expected error for private key file")
	}
}

func TestCertBackend_PathValidation(t *testing.T) {
	b := &CertBackend{allowedPaths: []string{"/tmp/grid"}}
	_, err := b.Run(context.Background(), "inspect", map[string]string{"path": "/etc/ssl/cert.pem"})
	if err == nil {
		t.Fatal("expected path validation error")
	}
}

func TestCertBackend_UnknownAction(t *testing.T) {
	b := &CertBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
