package backends

import "testing"

func TestBackendRegistry_AllRegistered(t *testing.T) {
	b := New()

	expected := []string{"ping", "test", "apt", "systemd", "rke2"}
	for _, name := range expected {
		if _, ok := b.Get(name); !ok {
			t.Errorf("backend %q not registered", name)
		}
	}
}

func TestBackendRegistry_List(t *testing.T) {
	b := New()
	list := b.List()

	if len(list) < 5 {
		t.Errorf("expected at least 5 backends, got %d", len(list))
	}

	// Verify known backends are in the list
	names := make(map[string]bool)
	for _, n := range list {
		names[n] = true
	}
	for _, want := range []string{"ping", "test", "apt", "systemd", "rke2"} {
		if !names[want] {
			t.Errorf("backend %q missing from List()", want)
		}
	}
}

func TestBackendRegistry_GetUnknown(t *testing.T) {
	b := New()
	if _, ok := b.Get("nonexistent"); ok {
		t.Error("Get(nonexistent) should return false")
	}
}
