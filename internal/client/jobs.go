package client

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/charmbracelet/log"
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
		Subject: fmt.Sprintf("job.all.%s.%s", job.Backend, job.Action),
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

	job.ID = pubAck.Sequence

	data, err := c.EncodeJob(job)
	if err != nil {
		log.Error("Error encoding job", "error", err)
		return err
	}

	log.Info("Storing job", "id", pubAck.Sequence, "backend", job.Backend, "action", job.Action)

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
