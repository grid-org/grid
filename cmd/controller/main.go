package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/controller"
	"github.com/urfave/cli/v2"
)

var (
	cfg *config.Config
	gc  *client.Client
)

func main() {
	app := &cli.App{
		Name:    "controller",
		Usage:   "Run the Grid Controller",
		Flags: common.AppFlags(),
		Before: setup,
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running Controller: %v", err)
	}
}

func setup(c *cli.Context) error {
	cfg = config.LoadConfig(c.String("config"))

	urls := c.String("urls")
	if urls != "" {
		cfg.NATS.URLS = []string{urls}
	}

	var err error
	gc, err = client.New(cfg)
	if err != nil {
		return err
	}
	return nil
}

func run(c *cli.Context) error {
	return controller.Run(cfg, gc)
}
