package main

import (
	"encoding/json"
	"os"

	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v2"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
)

var (
	cfg *config.Config
	gc  *client.Client
)

func main() {
	app := &cli.App{
		Name:   "grid",
		Usage:  "GRID client",
		Flags:  common.AppFlags(),
		Before: setup,
		Commands: []*cli.Command{
			{
				Name:  "status",
				Usage: "Cluster status",
				Action: getStatus,
			},
			{
				Name:  "job",
				Usage: "Create a new job",
				Subcommands: []*cli.Command{
					{
						Name:  "new",
						Usage: "Create a new job",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "action",
								Usage: "Action to perform",
							},
							&cli.StringFlag{
								Name:  "payload",
								Usage: "Payload to send",
							},
						},
						Action: newJob,
					},
					{
						Name:  "list",
						Usage: "List all jobs",
						Action: listJobs,
					},
					{
						Name:  "get",
						Usage: "Get a job",
						Args: true,
						Action: getJob,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running GRID: %v", err)
	}
}

func setup(c *cli.Context) error {
	cfg = config.LoadConfig(c.String("config"))
	var err error
	gc, err = client.New(cfg)
	if err != nil {
		return err
	}
	return nil
}

func getStatus(c *cli.Context) error {
	status, err := gc.GetClusterStatus()
	if err != nil {
		return err
	}
	log.Infof("Cluster status: %+v", status)
	return nil
}

func newJob(c *cli.Context) error {
	var req client.Request
	if err := json.Unmarshal([]byte(c.String("payload")), &req.Payload); err != nil {
		return err
	}

	req.Action = c.String("action")
	return gc.NewJob(req)
}

func listJobs(c *cli.Context) error {
	jobs, err := gc.ListJobs()
	if err != nil {
		return err
	}

	log.Infof("Jobs:")
	for _, entry := range jobs {
		log.Infof("%+v", entry)
	}

	return nil
}

func getJob(c *cli.Context) error {
	job, err := gc.GetJob(c.Args().First())
	if err != nil {
		return err
	}
	log.Infof("Job: %+v", job)
	return nil
}
