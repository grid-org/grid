package client

import (
	"encoding/json"
	"fmt"

	"github.com/grid-org/grid/internal/models"
)

// PutController stores or updates a controller entry in the controllers KV bucket.
func (c *Client) PutController(ctrl models.ControllerInfo) error {
	data, err := json.Marshal(ctrl)
	if err != nil {
		return fmt.Errorf("encoding controller: %w", err)
	}

	return c.PutKV("controllers", ctrl.ID, data)
}

// GetController retrieves a controller from the controllers KV bucket by ID.
func (c *Client) GetController(id string) (models.ControllerInfo, error) {
	entry, err := c.GetKV("controllers", id)
	if err != nil {
		return models.ControllerInfo{}, fmt.Errorf("getting controller %s: %w", id, err)
	}

	var ctrl models.ControllerInfo
	if err := json.Unmarshal(entry.Value(), &ctrl); err != nil {
		return models.ControllerInfo{}, fmt.Errorf("decoding controller %s: %w", id, err)
	}

	return ctrl, nil
}

// ListControllers returns all controllers from the controllers KV bucket.
func (c *Client) ListControllers() ([]models.ControllerInfo, error) {
	entries, err := c.ListKV("controllers")
	if err != nil {
		return nil, fmt.Errorf("listing controllers: %w", err)
	}

	controllers := make([]models.ControllerInfo, 0, len(entries))
	for _, entry := range entries {
		var ctrl models.ControllerInfo
		if err := json.Unmarshal(entry.Value(), &ctrl); err != nil {
			return nil, fmt.Errorf("decoding controller: %w", err)
		}
		controllers = append(controllers, ctrl)
	}

	return controllers, nil
}
