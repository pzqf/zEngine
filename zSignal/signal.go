package zSignal

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
)

// NotifyContext bridges process signals to a cancelable context. The returned
// function must be called to release the process-wide signal subscription.
func NotifyContext(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if len(signals) == 0 {
		signals = []os.Signal{syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT}
	}
	return signal.NotifyContext(parent, signals...)
}

// GracefulExit is kept for compatibility. New code should use NotifyContext
// and compose cancellation with its own lifecycle.
func GracefulExit() {
	ctx, release := NotifyContext(context.Background())
	defer release()
	<-ctx.Done()
	log.Printf("Receive shutdown signal, app will quit")
}
