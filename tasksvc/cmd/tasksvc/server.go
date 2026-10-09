package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// serve owns the listener and joins its single Serve goroutine before returning.
// Close releases sockets, not arbitrary handler code that ignores cancellation.
func serve(ctx context.Context, server *http.Server, listener net.Listener, shutdownTimeout time.Duration) error {
	if ctx.Err() != nil {
		return listener.Close()
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	normalize := func(err error) error {
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
	shutdown := func() error {
		// Fresh budget works after parent cancellation AND unexpected accept failure.
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownCtx)
		var closeErr error
		if shutdownErr != nil {
			closeErr = server.Close()
		}
		return errors.Join(shutdownErr, closeErr)
	}
	select {
	case err := <-result:
		// Serve closes the listener, but not existing accepted connections.
		return errors.Join(normalize(err), shutdown())
	case <-ctx.Done():
		shutdownErr := shutdown()
		serveErr := normalize(<-result)
		return errors.Join(shutdownErr, serveErr)
	}
}
