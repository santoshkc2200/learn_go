package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/tasksvc/internal/httpapi"
	"example.com/tasksvc/internal/store"
)

type config struct {
	Addr   string
	DBPath string
}

func parseConfig(args []string, output io.Writer) (config, error) {
	var cfg config
	flags := flag.NewFlagSet("tasksvc", flag.ContinueOnError)
	flags.SetOutput(output)
	flags.StringVar(&cfg.Addr, "addr", "127.0.0.1:8080", "literal loopback IP and port")
	flags.StringVar(&cfg.DBPath, "db", "tasks.db", "SQLite database filename")
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 0 {
		return config{}, errors.New("positional arguments are not supported")
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return config{}, errors.New("database path must not be empty")
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return config{}, fmt.Errorf("invalid address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return config{}, errors.New("address must use a literal loopback IP")
	}
	if port == "" || strings.IndexFunc(port, func(r rune) bool { return r < '0' || r > '9' }) != -1 {
		return config{}, errors.New("port must be numeric")
	}
	n, err := strconv.ParseUint(port, 10, 16)
	if err != nil || n > 65535 {
		return config{}, errors.New("port must be between 0 and 65535")
	}
	return cfg, nil
}

func run(ctx context.Context, args []string, output io.Writer, logger *slog.Logger) (code int) {
	if logger == nil {
		logger = slog.Default()
	}
	cfg, err := parseConfig(args, output)
	if errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(output, err)
		return 2
	}
	if ctx.Err() != nil {
		return 0
	}
	startupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := store.Open(startupCtx, cfg.DBPath)
	cancel()
	if err != nil {
		logger.Error("database startup failed", "error", err)
		return 1
	}
	// Registered before serving; execution occurs AFTER serve has stopped and joined.
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database", "error", err)
			code = 1
		}
	}()
	if ctx.Err() != nil {
		return 0
	}
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		logger.Error("listen failed", "error", err)
		return 1
	}
	server := &http.Server{
		Handler:           httpapi.New(store.New(db), logger),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20,
	}
	logger.Info("listening", "addr", listener.Addr().String(), "db", cfg.DBPath)
	if err := serve(ctx, server, listener, 5*time.Second); err != nil {
		logger.Error("server stopped with error", "error", err)
		return 1
	}
	logger.Info("server stopped")
	return 0
}
