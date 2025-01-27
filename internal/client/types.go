package client

import (
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Client struct {
	NC *nats.Conn
	JS jetstream.JetStream
}

// Some nats.Conn connection statuses
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

type Job struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}
