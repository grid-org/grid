package main

import (
	"encoding/json"
	"os"

	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v2"

	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
)

func main() {
	app := &cli.App{
		Name:  "grid",
		Usage: "GRID client",
		Flags: config.AppFlags(),
		Commands: []*cli.Command{
			{
				Name: "status",
				Usage: "Cluster status",
				Action: func(c *cli.Context) error {
					cfg := config.LoadConfig(c.String("config"))

					client, err := client.New(cfg)
					if err != nil {
						return err
					}

					return client.GetClusterStatus()
				},
			},
			{
				Name: "job",
				Usage: "Create a new job",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name: "action",
						Usage: "Action to perform",
					},
					&cli.StringFlag{
						Name: "payload",
						Usage: "Payload to send",
					},
				},
				Action: func(c *cli.Context) error {
					cfg := config.LoadConfig(c.String("config"))

					nc, err := client.New(cfg)
					if err != nil {
						return err
					}

					var req client.Request
					if err := json.Unmarshal([]byte(c.String("payload")), &req.Payload); err != nil {
						return err
					}

					req.Action = c.String("action")
					return nc.NewJob(req)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running GRID: %v", err)
	}
}
