package main

import (
	"context"
	"os"
	"os/signal"
)

func main() {
	// Ctrl+C cancels the same parent propagated to every job. Stop unregisters
	// notification; call it before os.Exit, which skips deferred cleanup.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	// os.Exit skips deferred functions. runCLI owns cleanup and completes it
	// before the process exit; reusable runner code never exits the process.
	code := runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
