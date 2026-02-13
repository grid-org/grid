package models

import (
	"fmt"
	"time"
)

// MaxPhaseDepth limits nesting depth. Top-level = depth 0 (barrier phases),
// one level of nesting = depth 1 (per-node pipelines).
const MaxPhaseDepth = 2

// Target specifies which nodes a job should execute on.
type Target struct {
	Scope string `json:"scope" yaml:"scope"` // "all" | "group" | "node"
	Value string `json:"value" yaml:"value"` // group name or node ID (empty for "all")
}

// Task is a single unit of work executed by one backend.
// Used as the command dispatch type (what gets sent over NATS to workers).
type Task struct {
	Backend    string            `json:"backend" yaml:"backend"`
	Action     string            `json:"action" yaml:"action"`
	Params     map[string]string `json:"params" yaml:"params"`
	Timeout    string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`       // per-task timeout (e.g. "30s", "5m")
	MaxRetries int               `json:"max_retries,omitempty" yaml:"max_retries,omitempty"` // max retry attempts on failure (0 = no retry)
	Condition  Condition         `json:"condition,omitempty" yaml:"condition,omitempty"`    // when to execute: always, on_success, on_failure
}

// Phase is a node in the hierarchical execution tree. It is either:
//   - A leaf: has Backend/Action (represents a single task)
//   - A branch: has Tasks (a pipeline of sub-phases executed per-node)
//
// Top-level phases are barrier-synchronized (all nodes sync between them).
// Nested phases within a branch are per-node pipelines (no cross-node sync).
type Phase struct {
	// Leaf fields (single task)
	Backend    string            `json:"backend,omitempty" yaml:"backend,omitempty"`
	Action     string            `json:"action,omitempty" yaml:"action,omitempty"`
	Params     map[string]string `json:"params,omitempty" yaml:"params,omitempty"`
	Timeout    string            `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`

	// Branch fields (pipeline of sub-phases)
	Tasks []Phase `json:"tasks,omitempty" yaml:"tasks,omitempty"`

	// Phase-level controls
	Condition Condition `json:"condition,omitempty" yaml:"condition,omitempty"`
}

// IsLeaf returns true if this phase represents a single task (has backend/action).
func (p Phase) IsLeaf() bool {
	return p.Backend != ""
}

// IsBranch returns true if this phase is a pipeline container (has sub-tasks).
func (p Phase) IsBranch() bool {
	return len(p.Tasks) > 0
}

// ToTask extracts a Task from a leaf Phase for command dispatch.
func (p Phase) ToTask() Task {
	return Task{
		Backend:    p.Backend,
		Action:     p.Action,
		Params:     p.Params,
		Timeout:    p.Timeout,
		MaxRetries: p.MaxRetries,
		Condition:  p.Condition,
	}
}

// LeafCount returns the total number of leaf tasks in this phase tree.
func (p Phase) LeafCount() int {
	if p.IsLeaf() {
		return 1
	}
	count := 0
	for _, sub := range p.Tasks {
		count += sub.LeafCount()
	}
	return count
}

// Validate checks that a phase is well-formed at the given depth.
func (p Phase) Validate(depth int) error {
	if depth >= MaxPhaseDepth {
		return fmt.Errorf("phase nesting exceeds maximum depth of %d", MaxPhaseDepth)
	}

	leaf := p.Backend != "" || p.Action != ""
	branch := len(p.Tasks) > 0

	if leaf && branch {
		return fmt.Errorf("phase cannot be both a leaf (backend/action) and a branch (tasks)")
	}
	if !leaf && !branch {
		return fmt.Errorf("phase must be either a leaf (backend/action) or a branch (tasks)")
	}

	if leaf {
		if p.Backend == "" {
			return fmt.Errorf("leaf phase missing backend")
		}
		if p.Action == "" {
			return fmt.Errorf("leaf phase missing action")
		}
	}

	if branch {
		for i, sub := range p.Tasks {
			if err := sub.Validate(depth + 1); err != nil {
				return fmt.Errorf("tasks[%d]: %w", i, err)
			}
		}
	}

	return nil
}

// ValidatePhases validates a list of top-level phases.
func ValidatePhases(phases []Phase) error {
	if len(phases) == 0 {
		return fmt.Errorf("at least one phase is required")
	}
	for i, p := range phases {
		if err := p.Validate(0); err != nil {
			return fmt.Errorf("phase[%d]: %w", i, err)
		}
	}
	return nil
}

// PhaseLeafCount returns the total leaf count across a slice of phases.
func PhaseLeafCount(phases []Phase) int {
	count := 0
	for _, p := range phases {
		count += p.LeafCount()
	}
	return count
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

// JobResults maps step index (as string) -> nodeID -> NodeResult.
type JobResults map[string]map[string]NodeResult

// Job is a sequence of phases with a target selector.
// Top-level phases are barrier-synchronized: all nodes complete phase N
// before phase N+1 begins. Nested phases within a pipeline branch execute
// per-node without cross-node synchronization.
type Job struct {
	ID        string     `json:"id"`
	Target    Target     `json:"target"`
	Tasks     []Phase    `json:"tasks"`
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
	Tasks    []Phase  `json:"tasks" yaml:"tasks"`
	Strategy Strategy `json:"strategy,omitempty" yaml:"strategy,omitempty"`
	Timeout  string   `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}
