package common

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

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
