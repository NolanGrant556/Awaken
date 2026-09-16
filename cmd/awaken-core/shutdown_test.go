package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCoreCleanupUsesOneDeadlineAndReportsUnfinishedOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	start := time.Now()
	err := joinCoreCleanup(ctx, func(received context.Context) error {
		if received != ctx {
			t.Error("owner got fresh deadline")
		}
		<-release
		return nil
	}, func(context.Context) error { return errors.New("fixture close failed") })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("false cleanup success: %v", err)
	}
	if !strings.Contains(err.Error(), "unfinished owners [owner-0]") {
		t.Fatalf("missing unfinished owner name: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("owner ignored total deadline")
	}
}

func TestNamedCoreCleanupReportsTimedOutOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	release := make(chan struct{})
	defer close(release)
	err := joinNamedCoreCleanup(ctx, cleanupOwner{"http", func(context.Context) error {
		<-release
		return nil
	}})
	if !strings.Contains(err.Error(), "unfinished owners [http]") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("http timeout must name the owner: %v", err)
	}
}

func TestCoreHTTPServerShutdownCancelsIdleSSE(t *testing.T) {
	started := make(chan struct{})
	srv := newCoreHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("missing flusher")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("idle SSE held shutdown for %s", elapsed)
	}
	if err := <-serveErr; err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve: %v", err)
	}
}

func TestCoreHTTPServerShutdownUnblocksUnreadBody(t *testing.T) {
	srv := newCoreHTTPServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		buf := make([]byte, 32*1024)
		for {
			if _, err := w.Write(buf); err != nil {
				return
			}
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
		}
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(ln) }()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "GET / HTTP/1.1\r\nHost: localhost\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("unread body held shutdown for %s", elapsed)
	}
	if err := <-serveErr; err != nil && err != http.ErrServerClosed {
		t.Fatalf("serve: %v", err)
	}
}
