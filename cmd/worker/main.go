package main

import (
	"github.com/alecthomas/kong"
	"github.com/grid-org/grid/internal/cli"
	"github.com/grid-org/grid/internal/common"
	"github.com/grid-org/grid/internal/worker"
)

type WorkerCLI struct {
	CLI *cli.CLI `embed:""`
}

func main() {
	app := &WorkerCLI{}
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

func (*WorkerCLI) Run(ctx *cli.Context) error {
	w := worker.New(ctx.Config, ctx.Client)
	if err := w.Start(); err != nil {
		return err
	}

	common.WaitForSignal()
	return nil
}
