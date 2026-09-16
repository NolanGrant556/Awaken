package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"sync"
	"time"
)

type cleanupOwner struct {
	name  string
	close func(context.Context) error
}

type coreHTTPServer struct {
	*http.Server
	cancel context.CancelFunc
	conns  sync.Map
}

// All owners share one deadline. Results are buffered so a late owner can exit.
func joinNamedCoreCleanup(ctx context.Context, owners ...cleanupOwner) error {
	type result struct {
		name string
		err  error
	}
	results := make(chan result, len(owners))
	pending := make(map[string]bool, len(owners))
	for _, owner := range owners {
		pending[owner.name] = true
		go func(owner cleanupOwner) {
			started := time.Now()
			err := owner.close(ctx)
			log.Printf("awaken-core shutdown owner=%s elapsed=%s error=%v", owner.name, time.Since(started).Round(time.Millisecond), err)
			results <- result{owner.name, err}
		}(owner)
	}
	var combined error
	for len(pending) > 0 {
		select {
		case result := <-results:
			delete(pending, result.name)
			if result.err != nil {
				combined = errors.Join(combined, fmt.Errorf("%s: %w", result.name, result.err))
			}
		case <-ctx.Done():
			names := make([]string, 0, len(pending))
			for name := range pending {
				names = append(names, name)
			}
			sort.Strings(names)
			return errors.Join(combined, fmt.Errorf("unfinished owners %v: %w", names, ctx.Err()))
		}
	}
	return combined
}

func joinCoreCleanup(ctx context.Context, owners ...func(context.Context) error) error {
	named := make([]cleanupOwner, len(owners))
	for i, owner := range owners {
		named[i] = cleanupOwner{fmt.Sprintf("owner-%d", i), owner}
	}
	return joinNamedCoreCleanup(ctx, named...)
}

// Request lifetime is independent of the drain deadline. Cancel and close
// connections only after run terminal records have been flushed, so SSE tails
// remain deliverable until then. Desktop may already have stopped reading those
// streams, so blocked writes must be aborted or Shutdown waits out the budget.
func newCoreHTTPServer(handler http.Handler) *coreHTTPServer {
	requests, cancel := context.WithCancel(context.Background())
	core := &coreHTTPServer{cancel: cancel}
	core.Server = &http.Server{
		Handler:     handler,
		BaseContext: func(net.Listener) context.Context { return requests },
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew, http.StateActive, http.StateIdle, http.StateHijacked:
				core.conns.Store(conn, struct{}{})
			case http.StateClosed:
				core.conns.Delete(conn)
			}
		},
	}
	return core
}

func (s *coreHTTPServer) expireTrackedConnections() {
	s.conns.Range(func(key, _ any) bool {
		_ = key.(net.Conn).SetDeadline(time.Now())
		return true
	})
}

func (s *coreHTTPServer) closeTrackedConnections() {
	s.conns.Range(func(key, _ any) bool {
		_ = key.(net.Conn).Close()
		s.conns.Delete(key)
		return true
	})
}

func (s *coreHTTPServer) Shutdown(ctx context.Context) error {
	s.SetKeepAlivesEnabled(false)
	s.cancel()
	s.expireTrackedConnections()
	err := s.Server.Shutdown(ctx)
	if err != nil {
		s.closeTrackedConnections()
		return errors.Join(err, s.Server.Close())
	}
	return nil
}
