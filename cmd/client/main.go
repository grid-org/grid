package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
	"github.com/grid-org/grid/internal/models"
)

type CLI struct {
	API    string    `name:"api" short:"a" help:"Controller API address" default:"http://localhost:8765" env:"GRID_API"`
	Debug  bool      `name:"debug" short:"d" help:"Enable debug logging"`
	Job    JobCmd    `cmd:"" help:"Job management"`
	Node   NodeCmd   `cmd:"" help:"Node management"`
	Status StatusCmd `cmd:"" help:"Get cluster status"`
}

// -- Job commands --

type JobCmd struct {
	Run    RunJobCmd    `cmd:"" help:"Submit a new job"`
	Get    GetJobCmd    `cmd:"" help:"Get job status"`
	List   ListJobCmd   `cmd:"" help:"List all jobs"`
	Cancel CancelJobCmd `cmd:"" help:"Cancel a running or pending job"`
}

type RunJobCmd struct {
	Target   string `name:"target" short:"t" help:"Target (all, group:<name>, node:<id>)" default:"all"`
	File     string `name:"file" short:"f" help:"Job file (YAML)" optional:""`
	Strategy string `name:"strategy" short:"s" help:"Failure strategy (fail-fast, continue)" optional:""`
	Timeout  string `name:"timeout" help:"Overall job timeout (e.g. 30m, 1h)" optional:""`
	Wait     bool   `name:"wait" short:"w" help:"Wait for job to complete and print result"`

	// Inline single-task shorthand: gridc job run -t all apt install package=curl
	Backend string   `arg:"" help:"Backend name" optional:""`
	Action  string   `arg:"" help:"Action name" optional:""`
	Params  []string `arg:"" help:"Params as key=value pairs" optional:""`
}

type GetJobCmd struct {
	ID string `arg:"" help:"Job ID"`
}

type CancelJobCmd struct {
	ID string `arg:"" help:"Job ID"`
}

type ListJobCmd struct{}

// -- Node commands --

type NodeCmd struct {
	List NodeListCmd `cmd:"" help:"List registered nodes"`
	Info NodeInfoCmd `cmd:"" help:"Get node details"`
}

type NodeListCmd struct{}

type NodeInfoCmd struct {
	ID string `arg:"" help:"Node ID"`
}

// -- Status --

type StatusCmd struct{}

func main() {
	app := &CLI{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:   true,
			FlagsLast: true,
			Summary:   true,
		}),
	)
	if app.Debug {
		log.SetLevel(log.DebugLevel)
	}
	ctx.FatalIfErrorf(ctx.Run(app))
}

// -- HTTP helpers --

func apiGet(api, path string) ([]byte, error) {
	resp, err := http.Get(api + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func apiPost(api, path string, payload any) ([]byte, int, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("encoding request: %w", err)
	}
	resp, err := http.Post(api+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return body, resp.StatusCode, nil
}

// -- Job run --

func (r *RunJobCmd) Run(app *CLI) error {
	target := parseTarget(r.Target)

	var tasks []models.Phase
	var strategy models.Strategy
	var timeout string

	if r.File != "" {
		data, err := os.ReadFile(r.File)
		if err != nil {
			return fmt.Errorf("reading job file: %w", err)
		}
		var jf models.JobFile
		if err := yaml.Unmarshal(data, &jf); err != nil {
			return fmt.Errorf("parsing job file: %w", err)
		}
		target = jf.Target
		tasks = jf.Tasks
		if jf.Strategy != "" {
			strategy = jf.Strategy
		}
		if jf.Timeout != "" {
			timeout = jf.Timeout
		}
	} else {
		if r.Backend == "" || r.Action == "" {
			return fmt.Errorf("provide backend and action, or use -f for a job file")
		}
		params := parseParams(r.Params)
		tasks = []models.Phase{{Backend: r.Backend, Action: r.Action, Params: params}}
	}

	// CLI flags override file values
	if r.Strategy != "" {
		strategy = models.Strategy(r.Strategy)
	}
	if strategy == "" {
		strategy = models.StrategyFailFast
	}
	if r.Timeout != "" {
		timeout = r.Timeout
	}

	reqBody := map[string]any{
		"target":   target,
		"tasks":    tasks,
		"strategy": strategy,
	}
	if timeout != "" {
		reqBody["timeout"] = timeout
	}

	body, status, err := apiPost(app.API, "/job", reqBody)
	if err != nil {
		return err
	}
	if status != http.StatusAccepted {
		return fmt.Errorf("API error (%d): %s", status, string(body))
	}

	var job models.Job
	if err := json.Unmarshal(body, &job); err != nil {
		fmt.Println(string(body))
		return nil
	}

	fmt.Printf("Job %s submitted (%d tasks, target=%s:%s, strategy=%s)\n",
		job.ID, len(job.Tasks), job.Target.Scope, job.Target.Value, job.Strategy)
	fmt.Printf("Expected nodes: %v\n", job.Expected)

	if !r.Wait {
		return nil
	}

	return waitForJob(app.API, job.ID)
}

func waitForJob(api, jobID string) error {
	fmt.Println("Waiting for completion...")

	for {
		body, err := apiGet(api, "/job/"+jobID)
		if err != nil {
			return err
		}

		var job models.Job
		if err := json.Unmarshal(body, &job); err != nil {
			return fmt.Errorf("parsing job: %w", err)
		}

		switch job.Status {
		case models.JobCompleted, models.JobFailed, models.JobCancelled:
			fmt.Printf("\nJob %s %s\n", job.ID, job.Status)
			data, _ := json.MarshalIndent(job.Results, "", "  ")
			fmt.Println(string(data))
			if job.Status != models.JobCompleted {
				os.Exit(1)
			}
			return nil
		}

		time.Sleep(500 * time.Millisecond)
	}
}

// -- Job cancel --

func (c *CancelJobCmd) Run(app *CLI) error {
	body, status, err := apiPost(app.API, "/job/"+c.ID+"/cancel", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("cancel failed (%d): %s", status, string(body))
	}

	fmt.Printf("Job %s cancelled\n", c.ID)
	return nil
}

// -- Job get --

func (g *GetJobCmd) Run(app *CLI) error {
	body, err := apiGet(app.API, "/job/"+g.ID)
	if err != nil {
		return err
	}

	var job models.Job
	if err := json.Unmarshal(body, &job); err != nil {
		fmt.Println(string(body))
		return nil
	}

	data, _ := json.MarshalIndent(job, "", "  ")
	fmt.Println(string(data))
	return nil
}

// -- Job list --

func (l *ListJobCmd) Run(app *CLI) error {
	body, err := apiGet(app.API, "/jobs")
	if err != nil {
		return err
	}

	var jobs []models.Job
	if err := json.Unmarshal(body, &jobs); err != nil {
		fmt.Println(string(body))
		return nil
	}

	if len(jobs) == 0 {
		fmt.Println("No jobs found")
		return nil
	}

	for _, job := range jobs {
		fmt.Printf("%-20s  %-10s  step %d/%d  target=%s:%s  tasks=%d\n",
			job.ID, job.Status, job.Step+1, len(job.Tasks),
			job.Target.Scope, job.Target.Value, len(job.Tasks))
	}
	return nil
}

// -- Node list --

func (n *NodeListCmd) Run(app *CLI) error {
	body, err := apiGet(app.API, "/nodes")
	if err != nil {
		return err
	}

	var nodes []models.NodeInfo
	if err := json.Unmarshal(body, &nodes); err != nil {
		fmt.Println(string(body))
		return nil
	}

	if len(nodes) == 0 {
		fmt.Println("No nodes registered")
		return nil
	}

	for _, node := range nodes {
		fmt.Printf("%-20s  %-8s  groups=%-20s  backends=%v\n",
			node.ID, node.Status, node.Groups, node.Backends)
	}
	return nil
}

// -- Node info --

func (n *NodeInfoCmd) Run(app *CLI) error {
	body, err := apiGet(app.API, "/node/"+n.ID)
	if err != nil {
		return err
	}

	var node models.NodeInfo
	if err := json.Unmarshal(body, &node); err != nil {
		fmt.Println(string(body))
		return nil
	}

	data, _ := json.MarshalIndent(node, "", "  ")
	fmt.Println(string(data))
	return nil
}

// -- Status --

func (s *StatusCmd) Run(app *CLI) error {
	body, err := apiGet(app.API, "/status")
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

// -- Helpers --

func parseTarget(s string) models.Target {
	if len(s) > 6 && s[:6] == "group:" {
		return models.Target{Scope: "group", Value: s[6:]}
	}
	if len(s) > 5 && s[:5] == "node:" {
		return models.Target{Scope: "node", Value: s[5:]}
	}
	return models.Target{Scope: "all"}
}

func parseParams(args []string) map[string]string {
	params := make(map[string]string)
	for _, arg := range args {
		for i, c := range arg {
			if c == '=' {
				params[arg[:i]] = arg[i+1:]
				break
			}
		}
	}
	return params
}
