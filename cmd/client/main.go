package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/goccy/go-yaml"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/models"
)

type CLI struct {
	Config string    `name:"config" short:"c" help:"Path to config file" default:"./config.yaml"`
	Debug  bool      `name:"debug" short:"d" help:"Enable debug logging"`
	Job    JobCmd    `cmd:"" help:"Job management"`
	Node   NodeCmd   `cmd:"" help:"Node management"`
	Status StatusCmd `cmd:"" help:"Get cluster status"`
}

// -- Job commands --

type JobCmd struct {
	Run  RunJobCmd  `cmd:"" help:"Submit a new job"`
	Get  GetJobCmd  `cmd:"" help:"Get job status"`
	List ListJobCmd `cmd:"" help:"List all jobs"`
}

type RunJobCmd struct {
	Target   string `name:"target" short:"t" help:"Target (all, group:<name>, node:<id>)" default:"all"`
	File     string `name:"file" short:"f" help:"Job file (YAML)" optional:""`
	Strategy string `name:"strategy" short:"s" help:"Failure strategy (fail-fast, continue)" default:"fail-fast"`
	API      string `name:"api" short:"a" help:"Controller API address" default:"http://localhost:8765"`

	// Inline single-task shorthand: gridc job run -t all apt install package=curl
	Backend string   `arg:"" help:"Backend name" optional:""`
	Action  string   `arg:"" help:"Action name" optional:""`
	Params  []string `arg:"" help:"Params as key=value pairs" optional:""`
}

type GetJobCmd struct {
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
	appCfg := &config.Config{}
	appClient := &client.Client{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:   true,
			FlagsLast: true,
			Summary:   true,
		}),
		kong.Bind(appCfg),
		kong.Bind(appClient),
	)
	ctx.FatalIfErrorf(ctx.Run(appCfg, appClient))
}

func (c *CLI) AfterApply(cfg *config.Config, cl *client.Client) error {
	if c.Debug {
		log.SetLevel(log.DebugLevel)
	}

	*cfg = *config.LoadConfig(c.Config)

	nc, err := client.New(cfg, nil)
	if err != nil {
		return err
	}
	*cl = *nc

	return nil
}

// -- Job run --

func (r *RunJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	target := parseTarget(r.Target)

	var tasks []models.Task

	if r.File != "" {
		// Load from file
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
	} else {
		// Build single-task from CLI args
		if r.Backend == "" || r.Action == "" {
			return fmt.Errorf("provide backend and action, or use -f for a job file")
		}
		params := parseParams(r.Params)
		tasks = []models.Task{{Backend: r.Backend, Action: r.Action, Params: params}}
	}

	// Submit via HTTP API
	reqBody := map[string]any{
		"target":   target,
		"tasks":    tasks,
		"strategy": r.Strategy,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("encoding request: %w", err)
	}

	resp, err := http.Post(r.API+"/job", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("submitting job to %s: %w", r.API, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var job models.Job
	if err := json.Unmarshal(respBody, &job); err != nil {
		fmt.Println(string(respBody))
		return nil
	}

	fmt.Printf("Job %s submitted (%d tasks, target=%s, strategy=%s)\n", job.ID, len(job.Tasks), r.Target, job.Strategy)
	fmt.Printf("Expected nodes: %v\n", job.Expected)
	return nil
}

// -- Job get --

func (g *GetJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	job, err := cl.GetJob(g.ID)
	if err != nil {
		return err
	}

	data, _ := json.MarshalIndent(job, "", "  ")
	fmt.Println(string(data))
	return nil
}

// -- Job list --

func (l *ListJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	jobs, err := cl.ListJobs()
	if err != nil {
		return err
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

func (n *NodeListCmd) Run(cfg *config.Config, cl *client.Client) error {
	nodes, err := cl.ListNodes()
	if err != nil {
		return err
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

func (n *NodeInfoCmd) Run(cfg *config.Config, cl *client.Client) error {
	node, err := cl.GetNode(n.ID)
	if err != nil {
		return err
	}

	data, _ := json.MarshalIndent(node, "", "  ")
	fmt.Println(string(data))
	return nil
}

// -- Status --

func (s *StatusCmd) Run(cfg *config.Config, cl *client.Client) error {
	status, err := cl.GetClusterStatus()
	if err != nil {
		return err
	}
	fmt.Println(string(status.Value()))
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
