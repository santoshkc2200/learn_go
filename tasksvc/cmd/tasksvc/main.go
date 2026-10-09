package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stderr, slog.Default())
	stop() // os.Exit does not run deferred functions
	os.Exit(code)
}
