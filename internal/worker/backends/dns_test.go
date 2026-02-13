package backends

import (
	"context"
	"encoding/json"
	"testing"
)

func TestDNSBackend_Actions(t *testing.T) {
	b := &DNSBackend{}
	want := map[string]bool{"lookup": true, "reverse": true, "resolvers": true, "check": true}
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

func TestDNSBackend_LookupLocalhost(t *testing.T) {
	b := &DNSBackend{}
	result, err := b.Run(context.Background(), "lookup", map[string]string{
		"name": "localhost",
		"type": "A",
	})
	if err != nil {
		t.Fatalf("lookup error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if data["name"] != "localhost" {
		t.Errorf("expected name=localhost, got %v", data["name"])
	}
}

func TestDNSBackend_InvalidType(t *testing.T) {
	b := &DNSBackend{}
	_, err := b.Run(context.Background(), "lookup", map[string]string{
		"name": "example.com",
		"type": "AXFR",
	})
	if err == nil {
		t.Fatal("expected error for AXFR type")
	}
}

func TestDNSBackend_Resolvers(t *testing.T) {
	b := &DNSBackend{}
	result, err := b.Run(context.Background(), "resolvers", nil)
	if err != nil {
		t.Fatalf("resolvers error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := data["resolvers"]; !ok {
		t.Error("missing 'resolvers' field")
	}
}

func TestDNSBackend_MissingParam(t *testing.T) {
	b := &DNSBackend{}
	_, err := b.Run(context.Background(), "lookup", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing name")
	}

	_, err = b.Run(context.Background(), "reverse", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing ip")
	}
}

func TestDNSBackend_UnknownAction(t *testing.T) {
	b := &DNSBackend{}
	_, err := b.Run(context.Background(), "invalid", nil)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
}
