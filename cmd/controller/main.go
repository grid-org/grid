package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/controller"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:    "grid-controller",
		Usage:   "Run the Grid Controller",
		Flags: config.AppFlags(),
		Action: func(c *cli.Context) error {
			cfg := config.LoadConfig(c.String("config"))

			client, err := client.New(cfg)
			if err != nil {
				return err
			}

			return controller.Run(cfg, client)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running Controller: %v", err)
	}
}
