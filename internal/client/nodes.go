package client

import (
	"encoding/json"
	"fmt"

	"github.com/grid-org/grid/internal/models"
)

// PutNode stores or updates a node entry in the nodes KV bucket.
func (c *Client) PutNode(node models.NodeInfo) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("encoding node: %w", err)
	}

	return c.PutKV("nodes", node.ID, data)
}

// GetNode retrieves a node from the nodes KV bucket by ID.
func (c *Client) GetNode(id string) (models.NodeInfo, error) {
	entry, err := c.GetKV("nodes", id)
	if err != nil {
		return models.NodeInfo{}, fmt.Errorf("getting node %s: %w", id, err)
	}

	var node models.NodeInfo
	if err := json.Unmarshal(entry.Value(), &node); err != nil {
		return models.NodeInfo{}, fmt.Errorf("decoding node %s: %w", id, err)
	}

	return node, nil
}

// ListNodes returns all nodes from the nodes KV bucket.
func (c *Client) ListNodes() ([]models.NodeInfo, error) {
	entries, err := c.ListKV("nodes")
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	nodes := make([]models.NodeInfo, 0, len(entries))
	for _, entry := range entries {
		var node models.NodeInfo
		if err := json.Unmarshal(entry.Value(), &node); err != nil {
			return nil, fmt.Errorf("decoding node: %w", err)
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}
