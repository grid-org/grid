package main

import (
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/worker"
	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v2"
	"os"
)

func main() {
	app := &cli.App{
		Name:    "grid-worker",
		Usage:   "Run the Grid Worker",
		Flags: config.AppFlags(),
		Action: func(c *cli.Context) error {
			cfg := config.LoadConfig(c.String("config"))

			client, err := client.New(cfg)
			if err != nil {
				return err
			}

			return worker.Run(cfg, client)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running Worker: %v", err)
	}
}
