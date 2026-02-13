package backends

import "testing"

func TestBackendRegistry_AllRegistered(t *testing.T) {
	b := New(nil)

	expected := []string{
		"ping", "test", "apt", "systemd", "rke2",
		"sysinfo", "file", "net", "proc", "journald",
		"health", "cert", "dns", "user", "firewall",
		"config", "package",
	}
	for _, name := range expected {
		if _, ok := b.Get(name); !ok {
			t.Errorf("backend %q not registered", name)
		}
	}
}

func TestBackendRegistry_List(t *testing.T) {
	b := New(nil)
	list := b.List()

	if len(list) < 17 {
		t.Errorf("expected at least 17 backends, got %d", len(list))
	}

	// Verify known backends are in the list
	names := make(map[string]bool)
	for _, n := range list {
		names[n] = true
	}
	for _, want := range []string{
		"ping", "test", "apt", "systemd", "rke2",
		"sysinfo", "file", "net", "proc", "journald",
		"health", "cert", "dns", "user", "firewall",
		"config", "package",
	} {
		if !names[want] {
			t.Errorf("backend %q missing from List()", want)
		}
	}
}

func TestBackendRegistry_GetUnknown(t *testing.T) {
	b := New(nil)
	if _, ok := b.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
}
