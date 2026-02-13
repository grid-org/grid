package client

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/models"
	"github.com/nats-io/nats.go"
)

// CreateJob stores a new job in the jobs KV bucket.
func (c *Client) CreateJob(job models.Job) (models.Job, error) {
	job.CreatedAt = time.Now().UTC()
	job.UpdatedAt = job.CreatedAt

	data, err := json.Marshal(job)
	if err != nil {
		return job, fmt.Errorf("encoding job: %w", err)
	}

	if err := c.PutKV("jobs", job.ID, data); err != nil {
		return job, fmt.Errorf("storing job: %w", err)
	}

	log.Info("Job created", "id", job.ID, "target", job.Target, "tasks", len(job.Tasks))
	return job, nil
}

// UpdateJob updates an existing job in the KV store.
func (c *Client) UpdateJob(job models.Job) error {
	job.UpdatedAt = time.Now().UTC()

	data, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("encoding job: %w", err)
	}

	return c.PutKV("jobs", job.ID, data)
}

// GetJob retrieves a job from the KV store by ID.
func (c *Client) GetJob(id string) (models.Job, error) {
	entry, err := c.GetKV("jobs", id)
	if err != nil {
		return models.Job{}, fmt.Errorf("getting job %s: %w", id, err)
	}

	var job models.Job
	if err := json.Unmarshal(entry.Value(), &job); err != nil {
		return models.Job{}, fmt.Errorf("decoding job %s: %w", id, err)
	}

	return job, nil
}

// ListJobs returns all jobs from the KV store.
func (c *Client) ListJobs() ([]models.Job, error) {
	entries, err := c.ListKV("jobs")
	if err != nil {
		return nil, fmt.Errorf("listing jobs: %w", err)
	}

	jobs := make([]models.Job, 0, len(entries))
	for _, entry := range entries {
		var job models.Job
		if err := json.Unmarshal(entry.Value(), &job); err != nil {
			return nil, fmt.Errorf("decoding job: %w", err)
		}
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// PublishCommand publishes a task command to the commands stream.
func (c *Client) PublishCommand(jobID string, taskIndex int, target models.Target, task models.Task) error {
	subject := BuildCommandSubject(target, task)

	params, err := json.Marshal(task.Params)
	if err != nil {
		return fmt.Errorf("encoding params: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Header: nats.Header{
			"job-id":     []string{jobID},
			"task-index": []string{fmt.Sprintf("%d", taskIndex)},
			"backend":    []string{task.Backend},
			"action":     []string{task.Action},
		},
		Data: params,
	}

	if _, err := c.Publish(msg); err != nil {
		return fmt.Errorf("publishing command: %w", err)
	}

	log.Debug("Command published", "subject", subject, "job", jobID, "task", taskIndex)
	return nil
}

// PublishCommandToNode publishes a task command to a specific node (for pipeline dispatch).
func (c *Client) PublishCommandToNode(jobID string, taskIndex int, nodeID string, task models.Task) error {
	subject := fmt.Sprintf("cmd.node.%s.%s.%s", nodeID, task.Backend, task.Action)

	params, err := json.Marshal(task.Params)
	if err != nil {
		return fmt.Errorf("encoding params: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Header: nats.Header{
			"job-id":     []string{jobID},
			"task-index": []string{fmt.Sprintf("%d", taskIndex)},
			"backend":    []string{task.Backend},
			"action":     []string{task.Action},
		},
		Data: params,
	}

	if _, err := c.Publish(msg); err != nil {
		return fmt.Errorf("publishing command to node: %w", err)
	}

	log.Debug("Command published to node", "subject", subject, "job", jobID, "task", taskIndex, "node", nodeID)
	return nil
}

// PublishResult publishes a task result to the results stream.
func (c *Client) PublishResult(result models.TaskResult) error {
	subject := fmt.Sprintf("result.%s.%d.%s", result.JobID, result.TaskIndex, result.NodeID)

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encoding result: %w", err)
	}

	msg := &nats.Msg{
		Subject: subject,
		Data:    data,
	}

	if _, err := c.Publish(msg); err != nil {
		return fmt.Errorf("publishing result: %w", err)
	}

	log.Debug("Result published", "subject", subject, "status", result.Status)
	return nil
}

// BuildCommandSubject constructs the NATS subject for a command.
// Format: cmd.<scope>[.<value>].<backend>.<action>
func BuildCommandSubject(target models.Target, task models.Task) string {
	switch target.Scope {
	case "group":
		return fmt.Sprintf("cmd.group.%s.%s.%s", target.Value, task.Backend, task.Action)
	case "node":
		return fmt.Sprintf("cmd.node.%s.%s.%s", target.Value, task.Backend, task.Action)
	default: // "all"
		return fmt.Sprintf("cmd.all.%s.%s", task.Backend, task.Action)
	}
}
