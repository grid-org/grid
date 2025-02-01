package client

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/grid-org/grid/internal/config"
)

type Client struct {
	nc *nats.Conn
	js jetstream.JetStream
}

type Status struct {
	Connected  bool            `json:"connected"`
	URL        string          `json:"url"`
	Address    string          `json:"address"`
	Statistics nats.Statistics `json:"statistics"`
}

type Request struct {
	ID        uint64         `json:"id"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

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
		nc: nc,
		js: js,
	}, nil
}

func (c *Client) GetClientStatus() Status {
	return Status{
		Connected:  c.nc.IsConnected(),
		URL:        c.nc.ConnectedUrl(),
		Address:    c.nc.ConnectedAddr(),
		Statistics: c.nc.Stats(),
	}
}

func (c *Client) Close() {
	c.nc.Close()
}

func (c *Client) EnsureStream(jscfg jetstream.StreamConfig) (jetstream.Stream, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := c.js.CreateOrUpdateStream(ctx, jscfg)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (c *Client) GetStream(name string) (jetstream.Stream, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := c.js.Stream(ctx, name)
	if err != nil {
		return nil, err
	}

	return stream, nil
}

func (c *Client) EnsureConsumer(stream jetstream.Stream, cfg jetstream.ConsumerConfig) (jetstream.Consumer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	consumer, err := stream.CreateOrUpdateConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return consumer, nil
}

func (c *Client) EnsureKV(cfg jetstream.KeyValueConfig) (jetstream.KeyValue, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bucket, err := c.js.CreateKeyValue(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return bucket, nil
}

func (c *Client) Publish(msg *nats.Msg) (*jetstream.PubAck, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ack, err := c.js.PublishMsg(ctx, msg)
	if err != nil {
		return nil, err
	}

	return ack, nil
}
