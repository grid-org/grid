package registry_test

import (
	"sort"
	"testing"
	"time"

	"github.com/grid-org/grid/internal/models"
	"github.com/grid-org/grid/internal/registry"
	"github.com/grid-org/grid/internal/testutil"
)

func TestNodeIDs(t *testing.T) {
	nodes := []models.NodeInfo{
		{ID: "a"},
		{ID: "b"},
		{ID: "c"},
	}
	ids := registry.NodeIDs(nodes)
	if len(ids) != 3 {
		t.Fatalf("len = %d, want 3", len(ids))
	}
	if ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Errorf("ids = %v, want [a b c]", ids)
	}
}

func TestNodeIDs_Empty(t *testing.T) {
	ids := registry.NodeIDs(nil)
	if len(ids) != 0 {
		t.Errorf("len = %d, want 0", len(ids))
	}
}

func seedNodes(t *testing.T, env *testutil.TestEnv) {
	t.Helper()
	env.RegisterNodes(t,
		testutil.OnlineNode("web-01", "web", "prod"),
		testutil.OnlineNode("web-02", "web", "prod"),
		testutil.OnlineNode("db-01", "db", "prod"),
	)
}

func TestResolve_All(t *testing.T) {
	env := testutil.NewTestEnv(t)
	seedNodes(t, env)

	reg := registry.New(env.Client)
	nodes, err := reg.Resolve(models.Target{Scope: "all"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	ids := sortedIDs(nodes)
	if len(ids) != 3 {
		t.Fatalf("len = %d, want 3", len(ids))
	}
}

func TestResolve_Group(t *testing.T) {
	env := testutil.NewTestEnv(t)
	seedNodes(t, env)

	reg := registry.New(env.Client)

	tests := []struct {
		group    string
		wantIDs  []string
	}{
		{"web", []string{"web-01", "web-02"}},
		{"db", []string{"db-01"}},
		{"prod", []string{"db-01", "web-01", "web-02"}},
	}

	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			nodes, err := reg.Resolve(models.Target{Scope: "group", Value: tt.group})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			ids := sortedIDs(nodes)
			sort.Strings(tt.wantIDs)
			if len(ids) != len(tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
			}
			for i := range ids {
				if ids[i] != tt.wantIDs[i] {
					t.Errorf("ids[%d] = %q, want %q", i, ids[i], tt.wantIDs[i])
				}
			}
		})
	}
}

func TestResolve_Node(t *testing.T) {
	env := testutil.NewTestEnv(t)
	seedNodes(t, env)

	reg := registry.New(env.Client)
	nodes, err := reg.Resolve(models.Target{Scope: "node", Value: "db-01"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(nodes) != 1 || nodes[0].ID != "db-01" {
		t.Errorf("nodes = %v, want [db-01]", registry.NodeIDs(nodes))
	}
}

func TestResolve_FiltersOfflineNodes(t *testing.T) {
	env := testutil.NewTestEnv(t)

	// Register one online and one offline node
	env.RegisterNodes(t,
		testutil.OnlineNode("active", "web"),
		models.NodeInfo{
			ID:       "stale",
			Groups:   []string{"web"},
			Status:   "online",
			LastSeen: time.Now().Add(-5 * time.Minute), // past threshold
		},
		models.NodeInfo{
			ID:       "offline",
			Groups:   []string{"web"},
			Status:   "offline",
			LastSeen: time.Now(),
		},
	)

	reg := registry.New(env.Client)
	nodes, err := reg.Resolve(models.Target{Scope: "all"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if len(nodes) != 1 || nodes[0].ID != "active" {
		t.Errorf("nodes = %v, want [active]", registry.NodeIDs(nodes))
	}
}

func TestResolve_NoMatch(t *testing.T) {
	env := testutil.NewTestEnv(t)
	seedNodes(t, env)

	reg := registry.New(env.Client)

	_, err := reg.Resolve(models.Target{Scope: "group", Value: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for unresolvable target")
	}
}

func sortedIDs(nodes []models.NodeInfo) []string {
	ids := registry.NodeIDs(nodes)
	sort.Strings(ids)
	return ids
}
