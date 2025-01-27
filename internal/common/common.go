package common

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v2"
)

func AppFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   "config.yaml",
			Usage:   "Load configuration from `FILE`",
			EnvVars: []string{"GRID_CONFIG"},
		},
		&cli.StringFlag{
			Name: "url",
			Aliases: []string{"u"},
			Usage: "NATS server URL",
			Value: "nats://localhost:4222",
			EnvVars: []string{"GRID_URL"},
		},
	}
}

func WaitForSignal() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	<-signalChan
}

func WaitForSignalWithCancel(ctx context.Context) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-signalChan:
	case <-ctx.Done():
	}
}
