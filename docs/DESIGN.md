# GRID Redesign: Orchestration Engine

## Overview

This document describes the redesign of GRID's messaging topology and job model.
The existing chassis is kept: embedded NATS server, Go package structure, backend
plugin registry, CLI framework, Docker/Compose tooling. What changes is how jobs
are defined, routed to workers, executed across multiple nodes, and reported on.

## Design Principles

1. **Commands, not shells** -- Backends define a closed set of actions with structured
   parameters. No arbitrary command execution. The security model depends on this.
2. **Fan-out is the default** -- "Run this on all web servers" is the primary use case,
   not "run this somewhere." Work-queue semantics are a special case of targeting a
   single node.
3. **Results are first-class** -- Every execution produces a result that flows back
   to the controller. Job status is derived from aggregated results, not message acks.
4. **The controller is a scheduler** -- It doesn't just create infrastructure. It
   resolves targets, dispatches commands, tracks progress, and manages multi-step
   execution.
5. **Nodes are self-describing** -- Workers register themselves with their hostname,
   group memberships, and available backends. The controller uses this for target
   resolution and validation.

## Architecture

```
                          ┌──────────────────────┐
                          │       Client         │
                          │  (CLI / HTTP / SDK)  │
                          └──────────┬───────────┘
                                     │
                              POST /job
                              GET /job/:id
                              GET /nodes
                                     │
                          ┌──────────▼───────────┐
                          │     Controller       │
                          │                      │
                          │  ┌────────────────┐  │
                          │  │ Embedded NATS  │  │
                          │  │ + JetStream    │  │
                          │  └────────────────┘  │
                          │                      │
                          │  ┌────────────────┐  │
                          │  │ Scheduler      │  │
                          │  │ - validates    │  │
                          │  │ - resolves     │  │
                          │  │   targets      │  │
                          │  │ - dispatches   │  │
                          │  │ - tracks       │  │
                          │  │   results      │  │
                          │  └────────────────┘  │
                          │                      │
                          │  ┌────────────────┐  │
                          │  │ API Server     │  │
                          │  └────────────────┘  │
                          └──────────┬───────────┘
                                     │
                    ┌────────────────┼────────────────┐
                    │                │                │
             ┌──────▼──────┐  ┌─────▼───────┐  ┌────▼────────┐
             │  Worker A   │  │  Worker B   │  │  Worker C   │
             │             │  │             │  │             │
             │ groups:     │  │ groups:     │  │ groups:     │
             │  [web,prod] │  │  [web,prod] │  │  [db,prod]  │
             │             │  │             │  │             │
             │ backends:   │  │ backends:   │  │ backends:   │
             │  [apt,      │  │  [apt,      │  │  [apt,      │
             │   systemd]  │  │   systemd]  │  │   systemd]  │
             └─────────────┘  └─────────────┘  └─────────────┘
```

## Data Model

### Task

A single unit of work executed by one backend.

```go
type Task struct {
    Backend string            `json:"backend"`
    Action  string            `json:"action"`
    Params  map[string]string `json:"params"`
}
```

Examples:
```json
{"backend": "apt",     "action": "install", "params": {"package": "curl"}}
{"backend": "systemd", "action": "restart", "params": {"unit": "nginx"}}
{"backend": "rke2",    "action": "start",   "params": {"mode": "server"}}
```

### Job

A sequence of tasks with a target selector. Tasks execute in order -- step N+1
does not begin until all targeted nodes have reported results for step N.

```go
type Job struct {
    ID        string    `json:"id"`
    Target    Target    `json:"target"`
    Tasks     []Task    `json:"tasks"`
    Status    string    `json:"status"`    // pending | running | completed | failed
    Step      int       `json:"step"`      // current task index
    Expected  []string  `json:"expected"`  // resolved node IDs
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type Target struct {
    Scope string `json:"scope"` // "all" | "group" | "node"
    Value string `json:"value"` // group name or node ID (empty for "all")
}
```

### TaskResult

Reported by a worker after executing a task.

```go
type TaskResult struct {
    JobID     string        `json:"job_id"`
    TaskIndex int           `json:"task_index"`
    NodeID    string        `json:"node_id"`
    Status    string        `json:"status"`   // "success" | "failed"
    Output    string        `json:"output"`
    Error     string        `json:"error,omitempty"`
    Duration  time.Duration `json:"duration"`
    Timestamp time.Time     `json:"timestamp"`
}
```

### NodeInfo

Self-reported by each worker on startup and via periodic heartbeat.

```go
type NodeInfo struct {
    ID        string    `json:"id"`
    Hostname  string    `json:"hostname"`
    Groups    []string  `json:"groups"`
    Backends  []string  `json:"backends"`
    Status    string    `json:"status"`    // "online" | "offline"
    LastSeen  time.Time `json:"last_seen"`
}
```

## NATS Infrastructure

### Streams

| Stream     | Retention | Policy    | Subjects    | Purpose                        |
|------------|-----------|-----------|-------------|--------------------------------|
| `commands` | Limits    | DiscardOld| `cmd.>`     | Controller dispatches to workers |
| `results`  | Limits    | DiscardOld| `result.>`  | Workers report back to controller |

Limits retention with a reasonable max age (e.g., 1 hour) and max bytes. Messages
are self-contained -- workers don't need to look up job definitions from KV.

The Request Stream from the original README (WorkQueue, `request.>`) is deferred
to Phase 2 for multi-controller HA. In Phase 1, the API handler calls the
scheduler directly in-process.

### KV Buckets

| Bucket  | Key Pattern            | Value         | Purpose                        |
|---------|------------------------|---------------|--------------------------------|
| `jobs`  | `<jobID>`              | Job JSON      | Job definitions and status     |
| `nodes` | `<nodeID>`             | NodeInfo JSON | Node registration and health   |

### Subject Hierarchy

```
Commands (controller → workers):
  cmd.all.<backend>.<action>           Target every node
  cmd.group.<name>.<backend>.<action>  Target a group
  cmd.node.<id>.<backend>.<action>     Target a specific node

Results (workers → controller):
  result.<jobID>.<nodeID>              Result from a specific node
```

### Worker Consumer Setup

Each worker creates a **durable pull consumer** on the `commands` stream with
filter subjects derived from its identity:

```go
FilterSubjects: []string{
    "cmd.all.>",                          // all broadcast commands
    "cmd.group.<group1>.>",               // each group it belongs to
    "cmd.group.<group2>.>",
    "cmd.node.<nodeID>.>",                // direct targeting
}
```

- **DeliverPolicy**: DeliverNew (don't replay old commands on restart)
- **AckPolicy**: AckExplicit
- **Durable**: `worker-<nodeID>`

Each worker independently receives and acks every matching command. This is fan-out
without requiring Interest retention -- each consumer tracks its own position.

## Backend Interface

```go
type Backend interface {
    // Run executes an action with the given parameters.
    // Context carries cancellation/timeout from the scheduler.
    Run(ctx context.Context, action string, params map[string]string) (*Result, error)

    // Actions returns the set of valid actions for this backend.
    Actions() []string
}

type Result struct {
    Output string
}
```

Changes from current:
- `context.Context` for cancellation and timeout propagation
- Structured `params` instead of a raw string payload
- `Actions()` for discovery and validation (controller can reject jobs
  referencing invalid backend/action combinations)
- Return `*Result` so output flows back through the results stream

### Example: APT Backend

```go
func (a *APTBackend) Actions() []string {
    return []string{"install", "remove", "update", "upgrade"}
}

func (a *APTBackend) Run(ctx context.Context, action string, params map[string]string) (*Result, error) {
    pkg := params["package"]
    if pkg == "" && (action == "install" || action == "remove") {
        return nil, fmt.Errorf("missing required param: package")
    }

    var cmd string
    switch action {
    case "install":
        cmd = fmt.Sprintf("apt-get install -y %s", shellescape.Quote(pkg))
    case "remove":
        cmd = fmt.Sprintf("apt-get remove -y %s", shellescape.Quote(pkg))
    case "update":
        cmd = "apt-get update"
    case "upgrade":
        cmd = "apt-get upgrade -y"
    }

    out, err := exec.CommandContext(ctx, "sh", "-c", cmd).CombinedOutput()
    return &Result{Output: string(out)}, err
}
```

Note: `shellescape.Quote` (or equivalent) is mandatory for any param interpolated
into a shell command. Backends that don't shell out (e.g., pure Go implementations,
API calls) don't need this.

## Controller Scheduler

The scheduler is the new core of the controller. It replaces the current
"create streams and wait" logic with an active orchestration loop.

### Job Lifecycle

```
1. Client submits job via API
       │
2. Scheduler validates:
   - All backends exist
   - All actions are valid for their backend
   - Target is resolvable (nodes exist in KV)
       │
3. Scheduler resolves target → set of node IDs
   - Stores Job in KV with status=running, expected=nodeIDs
       │
4. For each task (in order):
   │
   ├─ 4a. Publish command to commands stream
   │       Subject: cmd.<scope>.<value>.<backend>.<action>
   │       Headers: job-id, task-index
   │       Data: task params JSON
   │
   ├─ 4b. Watch results stream for result.<jobID>.*
   │       Track which expected nodes have reported
   │
   ├─ 4c. When all expected nodes report success → next task
   │       If any node reports failure → mark job failed, stop
   │       If timeout expires → mark job failed (with partial results)
   │
   └─ 4d. Update Job in KV with current step and intermediate results
       │
5. All tasks complete → update Job status to completed
```

### Scheduler Implementation Sketch

```go
type Scheduler struct {
    client *client.Client
    nodes  *NodeRegistry   // watches nodes KV bucket
}

func (s *Scheduler) Execute(job *Job) error {
    // Resolve targets
    nodes, err := s.nodes.Resolve(job.Target)
    if err != nil {
        return err
    }
    job.Expected = nodeIDs(nodes)
    job.Status = "running"
    s.client.PutJob(job)

    // Execute tasks sequentially
    for i, task := range job.Tasks {
        job.Step = i
        s.client.PutJob(job)

        // Dispatch
        subject := buildSubject(job.Target, task)
        s.client.PublishCommand(subject, job.ID, i, task)

        // Collect results with timeout
        results, err := s.collectResults(job.ID, i, job.Expected, 5*time.Minute)
        if err != nil {
            job.Status = "failed"
            s.client.PutJob(job)
            return err
        }

        // Check for failures
        for _, r := range results {
            if r.Status == "failed" {
                job.Status = "failed"
                s.client.PutJob(job)
                return fmt.Errorf("task %d failed on node %s: %s", i, r.NodeID, r.Error)
            }
        }
    }

    job.Status = "completed"
    s.client.PutJob(job)
    return nil
}
```

### Result Collection

The scheduler creates an ephemeral consumer on the `results` stream filtered
to `result.<jobID>.>` for each active job. It tracks received results against
the expected node set and completes when all nodes have reported (or timeout).

## Node Registration

### Worker Startup

```go
func (w *Worker) register() error {
    info := NodeInfo{
        ID:       w.config.NATS.Name,
        Hostname: hostname(),
        Groups:   w.config.Worker.Groups,
        Backends: w.backends.List(),
        Status:   "online",
        LastSeen: time.Now().UTC(),
    }
    data, _ := json.Marshal(info)
    return w.client.PutKV("nodes", info.ID, data)
}
```

### Heartbeat

Workers update their `LastSeen` timestamp periodically (e.g., every 30 seconds).
The controller can mark nodes as offline if `LastSeen` exceeds a threshold (e.g.,
2 minutes).

### Deregistration

On graceful shutdown, workers set their status to `"offline"` and stop their
consumer. On crash, the heartbeat timeout handles it.

## Worker Config Changes

```yaml
worker:
  groups: ["web", "production"]     # group memberships for targeting
  # backends are auto-discovered from the registry by default
```

Added to the existing config structure:

```go
type WorkerConfig struct {
    Groups []string `yaml:"groups"`
}

type Config struct {
    API    APIConfig    `yaml:"api"`
    NATS   NATSConfig   `yaml:"nats"`
    Worker WorkerConfig `yaml:"worker"`
}
```

## CLI Changes

```bash
# Submit a single-task job targeting all nodes
gridc job run --target all apt install --package curl

# Submit a single-task job targeting a group
gridc job run --target group:web systemd restart --unit nginx

# Submit a single-task job targeting a specific node
gridc job run --target node:web-01 rke2 start --mode server

# Submit a multi-task job from a file
gridc job run -f deploy-web.yaml

# Check job status (includes per-node results)
gridc job status <jobID>

# List jobs
gridc job list

# List registered nodes
gridc node list

# Show node details
gridc node info <nodeID>
```

### Job File Format

```yaml
target:
  scope: group
  value: web

tasks:
  - backend: apt
    action: update

  - backend: apt
    action: install
    params:
      package: nginx

  - backend: systemd
    action: enable
    params:
      unit: nginx

  - backend: systemd
    action: start
    params:
      unit: nginx
```

## API Changes

```
POST /job                    Submit a new job
GET  /job/:id                Get job status (includes per-node results)
GET  /jobs                   List all jobs (with filters)
GET  /nodes                  List registered nodes
GET  /node/:id               Get node details
```

### Example: POST /job

```json
{
  "target": {"scope": "group", "value": "web"},
  "tasks": [
    {"backend": "apt", "action": "install", "params": {"package": "curl"}},
    {"backend": "systemd", "action": "restart", "params": {"unit": "nginx"}}
  ]
}
```

### Example: GET /job/:id

```json
{
  "id": "job-a1b2c3",
  "target": {"scope": "group", "value": "web"},
  "tasks": [
    {"backend": "apt", "action": "install", "params": {"package": "curl"}},
    {"backend": "systemd", "action": "restart", "params": {"unit": "nginx"}}
  ],
  "status": "running",
  "step": 1,
  "expected": ["web-01", "web-02", "web-03"],
  "results": {
    "0": {
      "web-01": {"status": "success", "duration": "2.3s"},
      "web-02": {"status": "success", "duration": "1.8s"},
      "web-03": {"status": "success", "duration": "2.1s"}
    },
    "1": {
      "web-01": {"status": "success", "duration": "0.4s"},
      "web-02": {"status": "running"},
      "web-03": {"status": "pending"}
    }
  },
  "created_at": "2026-02-12T10:30:00Z"
}
```

## Message Flow: Complete Example

**Scenario**: Install curl on all web servers, then restart nginx.

```
1. Client: POST /job
   {target: {scope: "group", value: "web"}, tasks: [apt/install, systemd/restart]}

2. Controller/Scheduler:
   - Looks up nodes KV → web-01, web-02, web-03 are in group "web"
   - Stores job in KV: {id: "j1", status: "running", step: 0, expected: [...]}
   - Publishes to commands stream:
     Subject: cmd.group.web.apt.install
     Headers: {job-id: "j1", task-index: "0"}
     Data: {"package": "curl"}

3. Workers (web-01, web-02, web-03):
   - Each has consumer with filter "cmd.group.web.>"
   - Each receives the message independently
   - Each executes: apt-get install -y curl
   - Each publishes to results stream:
     Subject: result.j1.<nodeID>
     Data: {job_id: "j1", task_index: 0, node_id: "web-01", status: "success", ...}

4. Controller/Scheduler:
   - Watches result.j1.>
   - Receives 3 results, all success
   - Updates job: step=1
   - Publishes next command:
     Subject: cmd.group.web.systemd.restart
     Headers: {job-id: "j1", task-index: "1"}
     Data: {"unit": "nginx"}

5. Workers execute systemd restart, publish results.

6. Controller: all results in, updates job status to "completed".

7. Client: GET /job/j1 → sees full results per node per step.
```

## Implementation Phases

### Phase 1: Core Redesign

Changes to existing code:

| Component          | What Changes                                              |
|--------------------|-----------------------------------------------------------|
| `internal/client`  | New types (Job, Task, Target, TaskResult, NodeInfo).      |
|                    | New methods: PublishCommand, PublishResult, PutJob,       |
|                    | GetJob, WatchResults, PutNode, GetNode, ListNodes.        |
| `internal/config`  | Add WorkerConfig with Groups field.                       |
| `internal/worker`  | Node registration + heartbeat. New consumer setup with    |
|                    | group-based filters. Result publishing after execution.   |
| `backends`         | Updated interface: context, structured params, Actions(). |
|                    | Update apt, rke2, systemd. Remove host backend.          |
| `internal/controller` | Scheduler implementation. Commands stream + results    |
|                    | stream setup. Result aggregation loop.                    |
| `internal/api`     | New endpoints: POST /job, GET /job/:id, GET /nodes.      |
| `cmd/client`       | Updated CLI with --target, --params, job files.           |

New code:
- `internal/scheduler` -- the orchestration loop
- `internal/registry` -- node registration and target resolution

### Phase 2: Production Orchestration

- Request stream (WorkQueue) for decoupled job submission
- Job queuing and admission control
- Configurable failure strategies (fail-fast, continue-on-error, rollback)
- Job timeout configuration
- Retry policies per task
- Conditional task execution (only-if, on-failure)

### Phase 3: Production Hardening

- mTLS between all components
- Token auth with proper key management (NATS accounts + NKeys)
- Backend action allowlisting in controller config
- Config/template distribution via NATS Object Store
- Observability: Prometheus metrics, structured logging, OpenTelemetry traces
- Health check endpoints
- Graceful drain (worker finishes current task before shutdown)

## What We Keep As-Is

- `cmd/` entry points (minor CLI changes only)
- `internal/config` structure (additive changes)
- `internal/client` NATS connection logic
- `internal/common` signal handling
- Backend registry pattern (`init()` + `registerBackend`)
- Embedded NATS server in controller
- Docker/Compose/Taskfile tooling
- Kong CLI framework
- Echo HTTP framework
