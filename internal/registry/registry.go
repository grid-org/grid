package registry

import (
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/models"
)

const OfflineThreshold = 2 * time.Minute

// Registry manages node registration and target resolution.
type Registry struct {
	client *client.Client
}

func New(c *client.Client) *Registry {
	return &Registry{client: c}
}

// Resolve returns the set of node IDs that match a target selector.
// Only online nodes are included.
func (r *Registry) Resolve(target models.Target) ([]models.NodeInfo, error) {
	nodes, err := r.client.ListNodes()
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	var matched []models.NodeInfo
	for _, node := range nodes {
		if !r.isOnline(node) {
			continue
		}

		switch target.Scope {
		case "all":
			matched = append(matched, node)
		case "group":
			if hasGroup(node, target.Value) {
				matched = append(matched, node)
			}
		case "node":
			if node.ID == target.Value {
				matched = append(matched, node)
			}
		}
	}

	if len(matched) == 0 {
		return nil, fmt.Errorf("no online nodes match target %s:%s", target.Scope, target.Value)
	}

	log.Debug("Target resolved", "scope", target.Scope, "value", target.Value, "nodes", len(matched))
	return matched, nil
}

// NodeIDs extracts the IDs from a slice of NodeInfo.
func NodeIDs(nodes []models.NodeInfo) []string {
	ids := make([]string, len(nodes))
	for i, n := range nodes {
		ids[i] = n.ID
	}
	return ids
}

func (r *Registry) isOnline(node models.NodeInfo) bool {
	if node.Status != "online" {
		return false
	}
	return time.Since(node.LastSeen) < OfflineThreshold
}

func hasGroup(node models.NodeInfo, group string) bool {
	for _, g := range node.Groups {
		if g == group {
			return true
		}
	}
	return false
}
