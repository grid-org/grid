package client

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/config"
)

func New(cfg *config.Config) (*Client, error) {
	ncOpts := []nats.Option{
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.Name(cfg.NATS.Name),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			log.Errorf("Disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Info("Reconnected")
		}),
		nats.ClosedHandler(func(nc *nats.Conn) {
			log.Warn("Connection closed")
		}),
	}

	nc, err := nats.Connect(strings.Join(cfg.NATS.URLS, ","), ncOpts...)
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

func (c *Client) GetJob(id string) (Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucket, err := c.JS.KeyValue(ctx, "jobs")
	if err != nil {
		return Job{}, err
	}

	job, err := bucket.Get(ctx, id)
	if err != nil {
		return Job{}, err
	}

	var j Job
	if err := json.Unmarshal(job.Value(), &j); err != nil {
		return Job{}, err
	}

	return j, nil
}

func (c *Client) ListJobs() ([]Job, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucket, err := c.JS.KeyValue(ctx, "jobs")
	if err != nil {
		return nil, err
	}

	var jobs []Job
	lister, err := bucket.ListKeys(ctx)
	if err != nil {
		return nil, err
	}

	for entry := range lister.Keys() {
		var j Job
		if err := json.Unmarshal([]byte(entry), &j); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}

	return jobs, nil
}

func (c *Client) GetClusterStatus() (jetstream.KeyValueEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bucket, err := c.JS.KeyValue(ctx, "cluster")
	if err != nil {
		return nil, err
	}

	status, err := bucket.Get(ctx, "status")
	if err != nil {
		return nil, err
	}

	return status, nil
}
