package client

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Job struct {
	ID        uint64    `json:"id"`
	Backend   string    `json:"backend"`
	Action    string    `json:"action"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

func (c *Client) EncodeJob(job Job) ([]byte, error) {
	return json.Marshal(job)
}

func (c *Client) DecodeJob(job jetstream.KeyValueEntry) (Job, error) {
	var j Job
	if err := json.Unmarshal(job.Value(), &j); err != nil {
		return Job{}, err
	}

	return j, nil
}

func (c *Client) NewJob(job Job) error {
	job.Timestamp = time.Now().UTC()

	msg := &nats.Msg{
		Subject: "request.job",
		Header: nats.Header{
			"backend": []string{job.Backend},
			"action": []string{job.Action},
		},
		Data: []byte(job.Payload),
	}
	pubAck, err := c.Publish(msg)
	if err != nil {
		return err
	}

	data, err := c.EncodeJob(job)

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
