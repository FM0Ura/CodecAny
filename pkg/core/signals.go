package core

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// RunSignals cria um contexto cancelável que encerra ao receber SIGINT/SIGTERM
// (seção 10.6). O cancelamento aborta jobs e dispara a purga de resíduos.
func RunSignals(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case <-ctx.Done():
			signal.Stop(ch)
			return
		case <-ch:
			cancel()
			signal.Stop(ch)
		}
	}()
	return ctx, cancel
}
