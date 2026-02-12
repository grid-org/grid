package models

import "time"

// Target specifies which nodes a job should execute on.
type Target struct {
	Scope string `json:"scope" yaml:"scope"` // "all" | "group" | "node"
	Value string `json:"value" yaml:"value"` // group name or node ID (empty for "all")
}

// Task is a single unit of work executed by one backend.
type Task struct {
	Backend    string            `json:"backend" yaml:"backend"`
	Action     string            `json:"action" yaml:"action"`
	Params     map[string]string `json:"params" yaml:"params"`
	Timeout    string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`       // per-task timeout (e.g. "30s", "5m")
	MaxRetries int              `json:"max_retries,omitempty" yaml:"max_retries,omitempty"` // max retry attempts on failure (0 = no retry)
	Condition  Condition         `json:"condition,omitempty" yaml:"condition,omitempty"`    // when to execute: always, on_success, on_failure
}

// Condition controls when a task executes relative to prior step outcomes.
type Condition string

const (
	ConditionAlways    Condition = "always"     // run regardless of prior step outcome (default)
	ConditionOnSuccess Condition = "on_success" // run only if all prior steps succeeded
	ConditionOnFailure Condition = "on_failure" // run only if any prior step failed
)

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
	Attempts int          `json:"attempts,omitempty"` // number of attempts (>1 if retried)
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
	Timeout   string     `json:"timeout,omitempty"` // overall job timeout (e.g. "30m")
	Status    JobStatus  `json:"status"`
	Step      int        `json:"step"`      // current task index
	Expected  []string   `json:"expected"`  // resolved node IDs
	Results   JobResults `json:"results,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

const (
	JobCancelled JobStatus = "cancelled"
)

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
	ResultSkipped ResultStatus = "skipped"
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
	Target   Target   `json:"target" yaml:"target"`
	Tasks    []Task   `json:"tasks" yaml:"tasks"`
	Strategy Strategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}
