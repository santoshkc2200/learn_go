package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestServeGracefulShutdown(t *testing.T) { testHeldHandler(t, false) }
func TestServeShutdownDeadline(t *testing.T) { testHeldHandler(t, true) }
func testHeldHandler(t *testing.T, expire bool) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered := make(chan struct{})
	release := make(chan struct{})
	handlerDone := make(chan struct{})
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
		w.Write([]byte("done"))
		close(handlerDone)
	})}
	budget := time.Second
	if expire {
		budget = 25 * time.Millisecond
	}
	done := make(chan error, 1)
	go func() { done <- serve(ctx, server, listener, budget) }()
	client := &http.Client{Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	requestDone := make(chan error, 1)
	go func() {
		res, err := client.Get("http://" + listener.Addr().String())
		if err == nil {
			_, err = io.ReadAll(res.Body)
			res.Body.Close()
		}
		requestDone <- err
	}()
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
		cancel()
		server.Close()
		listener.Close()
	})
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("handler not entered")
	}
	cancel()
	if expire {
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("shutdown=%v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("shutdown did not enforce deadline")
		}
		select {
		case err := <-requestDone:
			if err == nil {
				t.Fatal("connection not force-closed")
			}
		case <-time.After(time.Second):
			t.Fatal("client still connected")
		}
		close(release)
		released = true
	} else {
		select {
		case err := <-done:
			t.Fatalf("returned before handler cleanup: %v", err)
		case <-time.After(30 * time.Millisecond):
		}
		close(release)
		released = true
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("serve not joined")
		}
		select {
		case err := <-requestDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("request not joined")
		}
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("handler not released")
	}
}
func TestServeAlreadyCanceled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := serve(ctx, &http.Server{}, listener, time.Second); err != nil {
		t.Fatal(err)
	}
	if conn, err := net.DialTimeout("tcp", listener.Addr().String(), 100*time.Millisecond); err == nil {
		conn.Close()
		t.Fatal("listener leaked")
	}
}

type brokenListener struct{ err error }

func (l brokenListener) Accept() (net.Conn, error) { return nil, l.err }
func (l brokenListener) Close() error              { return nil }
func (l brokenListener) Addr() net.Addr            { return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)} }
func TestServeUnexpectedError(t *testing.T) {
	cause := errors.New("broken listener")
	if err := serve(context.Background(), &http.Server{}, brokenListener{cause}, time.Second); !errors.Is(err, cause) {
		t.Fatalf("serve=%v", err)
	}
}

// The first Accept serves a real request; the next fails on a controlled barrier.
type failAfterAccept struct {
	net.Listener
	accepted bool
	fail     <-chan struct{}
	cause    error
}

func (l *failAfterAccept) Accept() (net.Conn, error) {
	if !l.accepted {
		conn, err := l.Listener.Accept()
		if err == nil {
			l.accepted = true
		}
		return conn, err
	}
	<-l.fail
	return nil, l.cause
}

func TestServeUnexpectedErrorWithActiveRequest(t *testing.T) {
	for _, expire := range []bool{false, true} {
		name := "graceful"
		if expire {
			name = "deadline"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			fail := make(chan struct{})
			release := make(chan struct{})
			entered := make(chan struct{})
			handlerDone := make(chan struct{})
			var failOnce, releaseOnce sync.Once
			cause := errors.New("permanent accept failure")
			server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				defer close(handlerDone)
				<-release
				w.Write([]byte("complete"))
			})}
			result := make(chan error, 1)
			serverDone := make(chan struct{})
			budget := time.Second
			if expire {
				budget = 25 * time.Millisecond
			}
			go func() {
				defer close(serverDone)
				result <- serve(context.Background(), server, &failAfterAccept{Listener: listener, fail: fail, cause: cause}, budget)
			}()
			client := &http.Client{Timeout: 2 * time.Second}
			requestResult := make(chan error, 1)
			requestDone := make(chan struct{})
			go func() {
				defer close(requestDone)
				response, err := client.Get("http://" + listener.Addr().String())
				if err == nil {
					_, err = io.ReadAll(response.Body)
					response.Body.Close()
				}
				requestResult <- err
			}()
			t.Cleanup(func() {
				failOnce.Do(func() { close(fail) })
				releaseOnce.Do(func() { close(release) })
				server.Close()
				listener.Close()
				client.CloseIdleConnections()
				for _, done := range []<-chan struct{}{serverDone, requestDone} {
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("cleanup goroutine not joined")
					}
				}
				// The handler may never have started if the connection itself failed.
				select {
				case <-entered:
					select {
					case <-handlerDone:
					case <-time.After(time.Second):
						t.Error("handler not joined")
					}
				default:
				}
			})
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not reach handler")
			}
			failOnce.Do(func() { close(fail) })
			if expire {
				select {
				case err := <-result:
					if !errors.Is(err, cause) || !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("shutdown must retain accept/deadline causes: %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("shutdown deadline not enforced")
				}
				select {
				case err := <-requestResult:
					if err == nil {
						t.Fatal("active connection not force-closed")
					}
				case <-time.After(time.Second):
					t.Fatal("active connection survived failure cleanup")
				}
				releaseOnce.Do(func() { close(release) })
			} else {
				select {
				case err := <-result:
					t.Fatalf("returned while handler active: %v", err)
				case <-time.After(30 * time.Millisecond):
				}
				releaseOnce.Do(func() { close(release) })
				select {
				case err := <-result:
					if !errors.Is(err, cause) {
						t.Fatalf("lost accept error: %v", err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("serve cleanup not joined")
				}
				select {
				case err := <-requestResult:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("request not completed")
				}
			}
		})
	}
}
