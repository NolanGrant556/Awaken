package outlook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancelQueuedDueMail(t *testing.T) {
	firstEntered := make(chan struct{})
	release := make(chan struct{})
	var secondSends atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "draft-1") {
			close(firstEntered)
			<-release
		}
		if strings.Contains(r.URL.Path, "draft-2") {
			secondSends.Add(1)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	client := NewClient(&fakeTokens{token: "audit-only"})
	client.SetHTTPClientForTest(server.Client(), server.URL)
	s := NewMailScheduler(client, t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	_, e := s.scheduleDraft("", "draft-1", now.Add(-time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.scheduleDraft("", "draft-2", now.Add(-time.Minute))
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	go func() { s.processDue(); close(done) }()
	<-firstEntered
	result, e := s.Cancel(context.Background(), second.ID, false)
	if e != nil {
		t.Fatal(e)
	}
	if result.ScheduledMail.State != "cancelled" {
		t.Fatal(result)
	}
	close(release)
	<-done
	if secondSends.Load() != 0 {
		t.Fatalf("cancel returned success, but queued draft was sent %d time(s)", secondSends.Load())
	}
}

func TestSendingClaimBlocksCancellationAndRestartReplay(t *testing.T) {
	s := NewMailScheduler(NewClient(&fakeTokens{token: "test-only"}), t.TempDir())
	job, err := s.scheduleDraft("", "draft", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, claimed, err := s.claimDue(job.ID); err != nil || !claimed {
		t.Fatalf("claim: %t %v", claimed, err)
	}
	if _, err := s.Cancel(context.Background(), job.ID, false); err == nil {
		t.Fatal("claimed send reported cancelled")
	}
	if _, claimed, err := s.claimDue(job.ID); err != nil || claimed {
		t.Fatalf("claim replayed: %t %v", claimed, err)
	}
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	jobs, err := s.List()
	if err != nil || len(jobs.Items) != 1 || jobs.Items[0].State != "failed" {
		t.Fatalf("restart lost unknown result: %+v %v", jobs, err)
	}
	if _, claimed, err := s.claimDue(job.ID); err != nil || claimed {
		t.Fatalf("restart replayed: %t %v", claimed, err)
	}
}
