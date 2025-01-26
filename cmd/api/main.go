package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v2"

	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
)

func main() {
	app := &cli.App{
		Name:  "grid-api",
		Usage: "Run the Grid API server",
		Flags: config.AppFlags(),
		Action: func(c *cli.Context) error {
			cfg := config.LoadConfig(c.String("config"))

			client, err := client.New(cfg)
			if err != nil {
				return err
			}

			return api.Run(cfg, client)
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running API server: %v", err)
	}
}
