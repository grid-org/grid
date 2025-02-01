package main

import (
	"context"
	"time"

	"github.com/alecthomas/kong"
	"github.com/grid-org/grid/internal/api"
	"github.com/grid-org/grid/internal/cli"
	"github.com/grid-org/grid/internal/common"
)

type APICLI struct {
	CLI *cli.CLI `embed:""`
}

func main() {
	app := &APICLI{}
	appCtx := &cli.Context{}
	ctx := kong.Parse(app,
		kong.ConfigureHelp(kong.HelpOptions{
			Compact:  true,
			FlagsLast: true,
			Summary:  true,
		}),
		kong.Bind(appCtx),
	)
	ctx.FatalIfErrorf(ctx.Run())
}

func (*APICLI) Run(ctx *cli.Context) error {
	server := api.New(ctx.Config, ctx.Client)
	e, err := server.Start()
	if err != nil {
		return err
	}

	common.WaitForSignal()

	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := e.Shutdown(shutdown); err != nil {
		return err
	}

	return nil
}
