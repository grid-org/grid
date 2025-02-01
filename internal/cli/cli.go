package cli

import (
	"github.com/charmbracelet/log"
	"github.com/grid-org/grid/internal/client"
	"github.com/grid-org/grid/internal/config"
)

type Context struct {
	Client *client.Client
	Config *config.Config
}

type CLI struct {
	Config string `name:"config" short:"c" help:"Path to config file" default:"./config.yaml"`
	Debug bool   `name:"debug" short:"d" help:"Enable debug logging"`
	URL   string `name:"url" short:"u" help:"NATS URL to connect to" default:"nats://localhost:4222"`
}

func (c *CLI) AfterApply(ctx *Context) error {
	if c.Debug {
		log.SetLevel(log.DebugLevel)
	}

	ctx.Config = config.LoadConfig(c.Config)
	if c.URL != "" {
		ctx.Config.NATS.URLS = []string{c.URL}
	}
	
	client, err := client.New(ctx.Config)
	if err != nil {
		return err
	}

	ctx.Client = client
	return nil
}
