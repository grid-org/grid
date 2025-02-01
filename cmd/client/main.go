package main

import (
	"encoding/json"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/cli"
	"github.com/grid-org/grid/internal/client"
)

type ClientCLI struct {
	CLI    *cli.CLI `embed:""`
	Job    JobCmd   `cmd:"" help:"Job management"`
	Status StatusCmd `cmd:"" help:"Get cluster status"`
}

type JobCmd struct {
	New  NewJobCmd  `cmd:"" help:"Create a new job"`
	List ListJobCmd `cmd:"" help:"List all jobs"`
	Get  GetJobCmd  `cmd:"" help:"Get a job"`
}

type NewJobCmd struct {
	Action  string `arg:"" help:"Action to perform"`
	Payload string `arg:"" help:"Payload to send"`
}

type ListJobCmd struct{}

type GetJobCmd struct {
	ID string `arg:"" help:"ID of the job to get"`
}

type StatusCmd struct{}

func main() {
	app := &ClientCLI{}
	appCtx := &cli.Context{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			FlagsLast: true,
			Summary: true,
		}),
		kong.Bind(appCtx),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func (n *NewJobCmd) Run(ctx *cli.Context) error {
	var req client.Request
	if err := json.Unmarshal([]byte(n.Payload), &req.Payload); err != nil {
		return err
	}

	req.Action = n.Action
	return ctx.Client.NewJob(req)
}

func (l *ListJobCmd) Run(ctx *cli.Context) error {
	jobs, err := ctx.Client.ListJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		log.Infof("Job %s: %s", job.ID, job.Action)
	}
	return nil
}

func (g *GetJobCmd) Run(ctx *cli.Context) error {
	job, err := ctx.Client.GetJob(g.ID)
	if err != nil {
		return err
	}
	log.Infof("Job %s: %s", job.ID, job.Action)
	return nil
}

func (s *StatusCmd) Run(ctx *cli.Context) error {
	status, err := ctx.Client.GetClusterStatus()
	if err != nil {
		return err
	}
	log.Info(string(status.Value()))
	return nil
}
