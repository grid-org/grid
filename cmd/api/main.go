package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/urfave/cli/v2"

	"github.com/grid-org/grid/internal/api"
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
		Name:  "api",
		Usage: "Run the Grid API server",
		Flags: common.AppFlags(),
		Before: setup,
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running API server: %v", err)
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

func run(c *cli.Context) error {
	return api.Run(cfg, gc)
}
