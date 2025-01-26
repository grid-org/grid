package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/config"
)

// func New(cfg *config.Config, clientOpts ...nats.Option) (*Client, error) {
func New(cfg *config.Config) (*Client, error) {
	nc, err := nats.Connect(cfg.NATS.URL, []nats.Option{
		nats.RetryOnFailedConnect(true),
	}...,
	)
	if err != nil {
		return nil, err
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, err
	}

	return &Client{
		NC: nc,
		JS: js,
	}, nil
}

func (c *Client) GetStatus() Status {
	return Status{
		Connected:  c.NC.IsConnected(),
		URL:        c.NC.ConnectedUrl(),
		Address:    c.NC.ConnectedAddr(),
		Statistics: c.NC.Stats(),
	}
}

func (c *Client) Close() {
	c.NC.Close()
}

func (c *Client) NewJob(req Request) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	data, err := json.Marshal(req.Payload)
	if err != nil {
		return err
	}

	c.JS.PublishMsg(ctx, &nats.Msg{
		Subject: "request.job",
		Header: nats.Header{
			"action": []string{req.Action},
		},
		Data: data,
	})
	return nil
}

func (c *Client) GetJob(id string) error {
	return nil
}

func (c *Client) GetClusterStatus() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bucket, err := c.JS.KeyValue(ctx, "cluster")
	if err != nil {
		return err
	}

	status, err := bucket.Get(ctx, "status")
	if err != nil {
		return err
	}

	log.Infof("Cluster status: %s", status)

	return nil
}
