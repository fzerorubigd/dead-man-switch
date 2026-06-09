// Package cli holds shared helpers for the life-signal CLIs.
package cli

import (
	"context"
	"os/signal"
	"syscall"
)

// Context returns a context cancelled on the usual termination signals,
// for use as the root context of a signal CLI.
func Context() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(),
		syscall.SIGKILL,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
		syscall.SIGABRT)
}
