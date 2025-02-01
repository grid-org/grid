package client

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Job struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Timestamp time.Time      `json:"timestamp"`
}

func (c *Client) DecodeJob(job jetstream.KeyValueEntry) (Job, error) {
	var j Job
	if err := json.Unmarshal(job.Value(), &j); err != nil {
		return Job{}, err
	}

	return j, nil
}

func (c *Client) NewJob(req Request) error {
	data, err := json.Marshal(req.Payload)
	if err != nil {
		return err
	}

	pubAck, err := c.Publish(&nats.Msg{
		Subject: "request.job",
		Header: nats.Header{
			"action": []string{req.Action},
		},
		Data: data,
	})
	if err != nil {
		return err
	}

	err = c.PutKV("jobs", strconv.FormatUint(pubAck.Sequence, 10), data)
	if err != nil {
		return err
	}

	return err
}

func (c *Client) GetJob(id string) (Job, error) {
	v, err := c.GetKV("jobs", id)
	if err != nil {
		return Job{}, err
	}

	return c.DecodeJob(v)
}

func (c *Client) ListJobs() ([]Job, error) {
	jobs, err := c.ListKV("jobs")
	if err != nil {
		return nil, err
	}

	var result []Job
	for _, job := range jobs {
		j, err := c.DecodeJob(job)
		if err != nil {
			return nil, err
		}

		result = append(result, j)
	}

	return result, nil
}
