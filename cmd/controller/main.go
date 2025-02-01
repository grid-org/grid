package main

import (
	"github.com/alecthomas/kong"
	"github.com/grid-org/grid/internal/cli"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/controller"
)

type ControllerCLI struct {
	CLI *cli.CLI `embed:""`
}

func main() {
	app := &ControllerCLI{}
	appCtx := &cli.Context{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact: true,
			FlagsLast: true,
			Summary: true,
		}),
		kong.Bind(appCtx),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func (*ControllerCLI) Run(ctx *cli.Context) error {
	c := controller.New(ctx.Config, ctx.Client)
	if err := c.Start(); err != nil {
		return err
	}

	common.WaitForSignal()
	return nil
}
