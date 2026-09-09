package agentbridge

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestPromptCancellationDuringCreate(t *testing.T) {
	entered := make(chan struct{})
	cleanup := make(chan struct{})
	cancelledHTTP := make(chan struct{})
	queried := make(chan struct{}, 1)
	var creating atomic.Bool
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/runs") {
			_, _ = io.Copy(io.Discard, r.Body)
			if requests.Add(1) == 1 {
				creating.Store(true)
				close(entered)
				<-r.Context().Done()
				close(cancelledHTTP)
				<-cleanup
				creating.Store(false)
				return
			}
			if creating.Load() {
				w.WriteHeader(409)
				return
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"run_id":"run-2"}`)
			return
		}
		if strings.Contains(r.URL.Path, "/events") {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"type\":\"RUN_FINISHED\",\"streamCursor\":1}\n\n")
			return
		}
		if r.Method == http.MethodGet {
			select {
			case queried <- struct{}{}:
			default:
			}
			fmt.Fprintf(w, `{"acp_session_id":"s1","run_starting":%t}`, creating.Load())
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	s := &bridgeSession{ctx: context.Background(), mapping: &ACPSessionMapping{ACPSessionID: "s1"}, coreClient: NewCoreClient(srv.URL, "test")}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type result struct {
		stop acpsdk.StopReason
		err  error
	}
	done := make(chan result, 1)
	go func() { stop, err := s.Prompt(ctx, "test"); done <- result{stop, err} }()
	<-entered
	cancel()
	select {
	case <-cancelledHTTP:
	case <-time.After(time.Second):
		close(cleanup)
		t.Fatal("cancel did not propagate to Core create request")
	}
	select {
	case <-queried:
	case <-time.After(time.Second):
		close(cleanup)
		t.Fatal("bridge did not check creation cleanup")
	}
	select {
	case got := <-done:
		close(cleanup)
		t.Fatalf("prompt returned before Core released reservation: %+v", got)
	default:
	}
	close(cleanup)
	select {
	case got := <-done:
		if got.err != nil || got.stop != acpsdk.StopReasonCancelled {
			t.Fatalf("cancel result: %+v", got)
		}
	case <-time.After(6 * time.Second):
		t.Fatal("prompt did not stop after cleanup")
	}
	if stop, err := s.Prompt(context.Background(), "next"); err != nil || stop != acpsdk.StopReasonEndTurn {
		t.Fatalf("immediate next prompt: stop=%s err=%v", stop, err)
	}
}
