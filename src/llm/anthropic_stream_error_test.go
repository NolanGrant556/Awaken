package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnthropicStreamErrorRejectsPartialResponse(t *testing.T) {
	for _, tool := range []bool{false, true} {
		t.Run(fmt.Sprint("tool=", tool), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				events := []string{`{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","content":[],"model":"test","usage":{"input_tokens":1,"output_tokens":0}}}`}
				if tool {
					events = append(events, `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"write_file","input":{}}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`)
				} else {
					events = append(events, `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"partial answer"}}`)
				}
				events = append(events, `{"type":"content_block_stop","index":0}`, `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)
				for _, e := range events {
					fmt.Fprintf(w, "data: %s\n\n", e)
				}
			}))
			defer server.Close()
			var completed, failed int
			resp, err := StreamInvokeWithObserver(context.Background(), anthropicConfig(server.URL), Request{UserPrompt: "test"}, StreamObserver{OnEvent: func(e StreamEvent) {
				if e.Type == StreamEventCompleted {
					completed++
				}
				if e.Type == StreamEventFailed {
					failed++
				}
			}})
			if err == nil || resp != nil || completed != 0 || failed != 1 {
				t.Fatalf("provider failure produced a response: response=%+v err=%v completed=%d failed=%d", resp, err, completed, failed)
			}
			if status, ok := RequestStatus(err); !ok || status != 529 {
				t.Fatalf("wrong error status: %v", err)
			}
		})
	}
}

func TestAnthropicStreamErrorClassificationAndRedaction(t *testing.T) {
	for _, tc := range []struct {
		code   string
		status int
		retry  bool
	}{{"overloaded_error", 529, true}, {"rate_limit_error", 429, true}, {"authentication_error", 401, false}, {"provider-private-secret", 0, false}, {"", 0, false}} {
		t.Run(tc.code, func(t *testing.T) {
			state := newAnthropicStreamState(Config{}, PreparedRequest{}, StreamObserver{})
			state.consume(fmt.Sprintf(`{"type":"error","error":{"type":%q,"message":"secret raw response"}}`, tc.code), "error")
			resp, err := state.response()
			if resp != nil || !errors.Is(err, ErrRequestFailed) {
				t.Fatalf("response=%+v err=%v", resp, err)
			}
			status, _ := RequestStatus(err)
			if status != tc.status || IsTransientRequestError(err) != tc.retry {
				t.Fatalf("status=%d retry=%v", status, IsTransientRequestError(err))
			}
			if strings.Contains(err.Error(), "secret") {
				t.Fatalf("raw provider error leaked: %v", err)
			}
		})
	}
}
