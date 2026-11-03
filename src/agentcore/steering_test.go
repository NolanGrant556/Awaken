package agentcore

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	conversation "awaken/src/context"
)

type runnerSteeringSink struct {
	pending []SteeringInput
}

func (*runnerSteeringSink) Emit(AwakenEvent) {}

func (s *runnerSteeringSink) TakeSteering(context.Context) ([]SteeringInput, error) {
	pending := s.pending
	s.pending = nil
	return pending, nil
}

func (s *runnerSteeringSink) SealSteering(allowContinuation bool) bool {
	return allowContinuation && len(s.pending) > 0
}

func TestRunnerSteeringContinuesAfterCandidateAnswer(t *testing.T) {
	sink := &runnerSteeringSink{}
	requests := 0
	server := newRunnerStreamServer(t, func(w http.ResponseWriter, req *http.Request) {
		requests++
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Error(err)
		}
		if requests == 1 {
			sink.pending = []SteeringInput{{ID: "insert", Text: "correct the output",
				Attachments: []conversation.MessageAttachment{
					{ID: "image", Kind: "image", MimeType: "image/png", DataBase64: "AQID"},
				}}}
			writeRunnerStreamText(t, w, "original candidate")
			return
		}
		for _, expected := range []string{"original candidate", "correct the output", "data:image/png;base64,AQID"} {
			if !strings.Contains(string(body), expected) {
				t.Errorf("second provider request omitted %q", expected)
			}
		}
		writeRunnerStreamText(t, w, "updated answer")
	})
	defer server.Close()
	cfg := testAgentConfig(server.URL, 3)
	cfg.ModelConfig.Capabilities.SupportsVision = true
	agent := NewAgent(cfg)
	answer, err := agent.RunStreamWithEvents(context.Background(), "start task", sink)
	if err != nil || answer != "updated answer" || requests != 2 {
		t.Fatalf("answer=%q calls=%d error=%v", answer, requests, err)
	}
}
