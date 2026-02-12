package models

import "time"

// Target specifies which nodes a job should execute on.
type Target struct {
	Scope string `json:"scope" yaml:"scope"` // "all" | "group" | "node"
	Value string `json:"value" yaml:"value"` // group name or node ID (empty for "all")
}

// Task is a single unit of work executed by one backend.
type Task struct {
	Backend string            `json:"backend" yaml:"backend"`
	Action  string            `json:"action" yaml:"action"`
	Params  map[string]string `json:"params" yaml:"params"`
}

// Strategy controls how the scheduler handles task failures.
type Strategy string

const (
	StrategyFailFast Strategy = "fail-fast" // stop on first failure (default)
	StrategyContinue Strategy = "continue"  // exclude failed nodes, continue with remaining
)

// NodeResult stores the outcome of a single task on a single node.
type NodeResult struct {
	Status   ResultStatus `json:"status"`
	Output   string       `json:"output,omitempty"`
	Error    string       `json:"error,omitempty"`
	Duration string       `json:"duration"`
}

// JobResults maps step index (as string) → nodeID → NodeResult.
type JobResults map[string]map[string]NodeResult

// Job is a sequence of tasks with a target selector.
// Tasks execute in order: step N+1 does not begin until all targeted nodes
// have reported results for step N.
type Job struct {
	ID        string     `json:"id"`
	Target    Target     `json:"target"`
	Tasks     []Task     `json:"tasks"`
	Strategy  Strategy   `json:"strategy"`
	Status    JobStatus  `json:"status"`
	Step      int        `json:"step"`      // current task index
	Expected  []string   `json:"expected"`  // resolved node IDs
	Results   JobResults `json:"results,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// JobStatus represents the lifecycle state of a job.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

// TaskResult is reported by a worker after executing a task.
type TaskResult struct {
	JobID     string        `json:"job_id"`
	TaskIndex int           `json:"task_index"`
	NodeID    string        `json:"node_id"`
	Status    ResultStatus  `json:"status"`
	Output    string        `json:"output"`
	Error     string        `json:"error,omitempty"`
	Duration  time.Duration `json:"duration"`
	Timestamp time.Time     `json:"timestamp"`
}

// ResultStatus represents the outcome of a single task execution on one node.
type ResultStatus string

const (
	ResultSuccess ResultStatus = "success"
	ResultFailed  ResultStatus = "failed"
)

// NodeInfo is self-reported by each worker on startup and via heartbeat.
type NodeInfo struct {
	ID        string    `json:"id"`
	Hostname  string    `json:"hostname"`
	Groups    []string  `json:"groups"`
	Backends  []string  `json:"backends"`
	Status    string    `json:"status"` // "online" | "offline"
	LastSeen  time.Time `json:"last_seen"`
}

// JobFile is the YAML structure for submitting multi-task jobs from a file.
type JobFile struct {
	Target Target `json:"target" yaml:"target"`
	Tasks  []Task `json:"tasks" yaml:"tasks"`
}
