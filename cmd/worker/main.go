package main

import (
	"github.com/alecthomas/kong"
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/config"
	"github.com/grid-org/grid/internal/worker"
)

type CLI struct {
	Config string `name:"config" short:"c" help:"Path to config file" default:"./config.yaml"`
	Debug  bool   `name:"debug" short:"d" help:"Enable debug logging"`
}

func main() {
	app := &CLI{}
	appCfg := &config.Config{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:   true,
			FlagsLast: true,
			Summary:   true,
		}),
		kong.Bind(appCfg),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func (c *CLI) AfterApply(cfg *config.Config) error {
	if c.Debug {
		log.SetLevel(log.DebugLevel)
	}

	*cfg = *config.LoadConfig(c.Config)
	return nil
}

func (c *CLI) Run(cfg *config.Config) error {
	w := worker.New(cfg)
	if err := w.Start(); err != nil {
		return err
	}

	return nil
}
