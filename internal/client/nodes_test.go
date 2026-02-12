package client_test

import (
	"testing"
	"time"

	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/testutil"
)

func TestPutGetNode(t *testing.T) {
	env := testutil.NewTestEnv(t)

	node := models.NodeInfo{
		ID:       "node-1",
		Hostname: "server1.local",
		Groups:   []string{"web", "prod"},
		Backends: []string{"apt", "systemd"},
		Status:   "online",
		LastSeen: time.Now().UTC(),
	}

	if err := env.Client.PutNode(node); err != nil {
		t.Fatalf("PutNode: %v", err)
	}

	got, err := env.Client.GetNode("node-1")
	if err != nil {
		t.Fatalf("GetNode: %v", err)
	}

	if got.ID != "node-1" {
		t.Errorf("ID = %q, want node-1", got.ID)
	}
	if got.Hostname != "server1.local" {
		t.Errorf("Hostname = %q, want server1.local", got.Hostname)
	}
	if len(got.Groups) != 2 {
		t.Errorf("Groups len = %d, want 2", len(got.Groups))
	}
	if got.Status != "online" {
		t.Errorf("Status = %q, want online", got.Status)
	}
}

func TestGetNode_NotFound(t *testing.T) {
	env := testutil.NewTestEnv(t)

	_, err := env.Client.GetNode("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent node")
	}
}

func TestListNodes(t *testing.T) {
	env := testutil.NewTestEnv(t)

	for _, id := range []string{"n1", "n2", "n3"} {
		node := models.NodeInfo{
			ID:       id,
			Status:   "online",
			LastSeen: time.Now().UTC(),
		}
		if err := env.Client.PutNode(node); err != nil {
			t.Fatalf("PutNode(%s): %v", id, err)
		}
	}

	nodes, err := env.Client.ListNodes()
	if err != nil {
		t.Fatalf("ListNodes: %v", err)
	}

	if len(nodes) != 3 {
		t.Errorf("len = %d, want 3", len(nodes))
	}
}
