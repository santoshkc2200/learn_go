package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"example.com/tasksvc/internal/tasks"
)

func TestParseConfig(t *testing.T) {
	cfg, err := parseConfig(nil, io.Discard)
	if err != nil || cfg.Addr != "127.0.0.1:8080" || cfg.DBPath != "tasks.db" {
		t.Fatalf("defaults=%+v, %v", cfg, err)
	}
	for _, addr := range []string{"127.0.0.1:0", "127.9.8.7:65535", "[::1]:8080"} {
		t.Run(addr, func(t *testing.T) {
			cfg, err := parseConfig([]string{"-addr", addr, "-db", "my.db"}, io.Discard)
			if err != nil || cfg.Addr != addr || cfg.DBPath != "my.db" {
				t.Fatalf("config=%+v err=%v", cfg, err)
			}
		})
	}
	for _, args := range [][]string{{"-addr", "0.0.0.0:8080"}, {"-addr", ":8080"}, {"-addr", "localhost:8080"}, {"-addr", "192.0.2.1:80"}, {"-addr", "[::]:8080"}, {"-addr", "127.0.0.1:65536"}, {"-addr", "127.0.0.1:-1"}, {"-addr", "127.0.0.1:http"}, {"-addr", "127.0.0.1:+80"}, {"-db", ""}, {"extra"}, {"-unknown"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			if _, err := parseConfig(args, io.Discard); err == nil {
				t.Fatal("accepted invalid flags")
			}
		})
	}
	if _, err := parseConfig([]string{"-h"}, io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help=%v", err)
	}
}
func TestRunEarlyExit(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if got := run(context.Background(), []string{"-h"}, io.Discard, logger); got != 0 {
		t.Fatalf("help=%d", got)
	}
	if got := run(context.Background(), []string{"-bad"}, io.Discard, logger); got != 2 {
		t.Fatalf("bad flags=%d", got)
	}
	path := filepath.Join(t.TempDir(), "never.db")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := run(ctx, []string{"-db", path, "-addr", "127.0.0.1:0"}, io.Discard, logger); got != 0 {
		t.Fatalf("canceled=%d", got)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canceled startup created file: %v", err)
	}
	if got := run(context.Background(), []string{"-db", filepath.Join(t.TempDir(), "missing", "tasks.db")}, io.Discard, logger); got != 1 {
		t.Fatalf("startup failure=%d", got)
	}
}

// Capture a structured startup event without parsing log presentation text.
type addressHandler struct{ addresses chan string }

func (h addressHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h addressHandler) Handle(_ context.Context, r slog.Record) error {
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "addr" {
			select {
			case h.addresses <- a.Value.String():
			default:
			}
		}
		return true
	})
	return nil
}
func (h addressHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h addressHandler) WithGroup(string) slog.Handler      { return h }

func TestRunSmoke(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addresses := make(chan string, 1)
	done := make(chan int, 1)
	path := filepath.Join(t.TempDir(), "smoke.db")
	go func() {
		done <- run(ctx, []string{"-addr", "127.0.0.1:0", "-db", path}, io.Discard, slog.New(addressHandler{addresses}))
	}()
	// Cleanup stops and joins even when a request assertion fails.
	t.Cleanup(func() {
		cancel()
		select {
		case code := <-done:
			if code != 0 {
				t.Errorf("run exit=%d", code)
			}
		case <-time.After(7 * time.Second):
			t.Error("run did not stop")
		}
	})
	var addr string
	select {
	case addr = <-addresses:
	case code := <-done:
		done <- code
		t.Fatalf("run exited before startup: %d", code)
	case <-time.After(3 * time.Second):
		t.Fatal("no startup address")
	}
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Post("http://"+addr+"/tasks", "application/json", strings.NewReader(`{"title":"smoke"}`))
	if err != nil {
		t.Fatal(err)
	}
	var created tasks.Task
	err = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	if err != nil || response.StatusCode != 201 || created.ID <= 0 {
		t.Fatalf("POST=%+v status=%d err=%v", created, response.StatusCode, err)
	}
	response, err = client.Get("http://" + addr + response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	var got tasks.Task
	err = json.NewDecoder(response.Body).Decode(&got)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || got != created {
		t.Fatalf("GET=%+v status=%d err=%v", got, response.StatusCode, err)
	}
	response, err = client.Get("http://" + addr + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	var list struct{ Tasks []tasks.Task }
	err = json.NewDecoder(response.Body).Decode(&list)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || len(list.Tasks) != 1 || list.Tasks[0] != created {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}
