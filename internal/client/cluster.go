package client

import (
	"github.com/nats-io/nats.go/jetstream"
)

func (c *Client) GetClusterStatus() (jetstream.KeyValueEntry, error) {
	status, err := c.GetKV("cluster", "status")
	if err != nil {
		return nil, err
	}

	return status, nil
}
