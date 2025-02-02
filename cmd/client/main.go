package main

import (
	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
)

type CLI struct {
	Config string    `name:"config" short:"c" help:"Path to config file" default:"./config.yaml"`
	Debug  bool      `name:"debug" short:"d" help:"Enable debug logging"`
	Job    JobCmd    `cmd:"" help:"Job management"`
	Status StatusCmd `cmd:"" help:"Get cluster status"`
}

type JobCmd struct {
	New  NewJobCmd  `cmd:"" help:"Create a new job"`
	List ListJobCmd `cmd:"" help:"List all jobs"`
	Get  GetJobCmd  `cmd:"" help:"Get a job"`
}

type NewJobCmd struct {
	Backend string `arg:"" help:"Backend to use"`
	Action  string `arg:"" help:"Action to perform"`
	Payload string `arg:"" help:"Payload to send" optional:""`
}

type ListJobCmd struct{}

type GetJobCmd struct {
	ID string `arg:"" help:"ID of the job to get"`
}

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

	var err error
	nc, err := client.New(cfg, nil)
	if err != nil {
		return err
	}
	*cl = *nc

	return nil
}

func (n *NewJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	job := client.Job{
		Backend: n.Backend,
		Action:  n.Action,
		Payload: n.Payload,
	}
	return cl.NewJob(job)
}

func (l *ListJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	jobs, err := cl.ListJobs()
	if err != nil {
		return err
	}

	for _, job := range jobs {
		log.Infof("Job %+v", job)
	}
	return nil
}

func (g *GetJobCmd) Run(cfg *config.Config, cl *client.Client) error {
	job, err := cl.GetJob(g.ID)
	if err != nil {
		return err
	}
	log.Infof("Job %d: %s", job.ID, job.Action)
	return nil
}

func (s *StatusCmd) Run(cfg *config.Config, cl *client.Client) error {
	status, err := cl.GetClusterStatus()
	if err != nil {
		return err
	}
	log.Info(string(status.Value()))
	return nil
}
