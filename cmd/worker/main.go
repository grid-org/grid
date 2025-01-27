package main

import (
	"os"

	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/worker"
	"github.com/urfave/cli/v2"
)

var (
	cfg *config.Config
	gc  *client.Client
)

func main() {
	app := &cli.App{
		Name:    "worker",
		Usage:   "Run the Grid Worker",
		Flags: common.AppFlags(),
		Before: setup,
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Error running Worker: %v", err)
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
	return worker.Run(cfg, gc)
}
