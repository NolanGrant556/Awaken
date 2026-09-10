package subagentruntime

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestAcceptanceDefaultCapacityIncludesRootAndRejectsQueue129(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started := make(chan string, 140)
	m := NewManager(ctx, Config{Starter: func(child context.Context, req StartRequest) (string, error) {
		started <- req.Task.TaskID
		<-child.Done()
		return "", child.Err()
	}})
	defer m.Close(context.Background())
	root, leave, err := m.EnterExecution(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	for i := 0; i < 7; i++ {
		if _, err := m.Spawn(root, SpawnRequest{Description: fmt.Sprintf("active-%d", i), Prompt: "hold"}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("active child did not start")
		}
	}
	for i := 0; i < 128; i++ {
		task, err := m.Spawn(root, SpawnRequest{Description: fmt.Sprintf("queued-%d", i), Prompt: "must remain queued"})
		if err != nil {
			t.Fatalf("queue item %d rejected: %v", i+1, err)
		}
		if task.Status != StatusQueued {
			t.Fatalf("queue item %d: %s", i+1, task.Status)
		}
	}
	_, err = m.Spawn(root, SpawnRequest{Description: "overflow", Prompt: "must reject"})
	var issue *Error
	if !errors.As(err, &issue) || issue.Code != "subagent_concurrency_queue_full" {
		t.Fatalf("129th queued assignment: %v", err)
	}
	select {
	case id := <-started:
		t.Fatalf("root plus eight children exceeded eight slots: %s", id)
	default:
	}
	if err := m.StopAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.EndAttempt(ctx, "root", ""); err != nil {
		t.Fatal(err)
	}
	if m.HasLiveWork("") || len(m.List()) != 135 {
		t.Fatalf("lost tasks or unfinished cleanup: count=%d", len(m.List()))
	}
	for _, task := range m.List() {
		if task.Status != StatusCancelled {
			t.Fatalf("task %s status=%s", task.TaskID, task.Status)
		}
	}
	select {
	case id := <-started:
		t.Fatalf("queued child started during cancellation: %s", id)
	default:
	}
}

func TestAcceptanceDefaultDepthAllowsEightAndRejectsNine(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var starts atomic.Int32
	var m *Manager
	var starter Starter
	starter = func(child context.Context, req StartRequest) (string, error) {
		starts.Add(1)
		depth, err := strconv.Atoi(req.Prompt)
		if err != nil {
			return "", err
		}
		controller := m.Bind(child, req.Task.AgentID, depth, Config{Starter: starter})
		next, err := controller.Spawn(child, SpawnRequest{Description: fmt.Sprintf("depth-%d", depth+1), Prompt: strconv.Itoa(depth + 1)})
		if depth == 8 {
			var issue *Error
			if !errors.As(err, &issue) || issue.Code != ErrorLimitReached {
				return "", fmt.Errorf("depth nine accepted: %v", err)
			}
			return "depth-eight-confirmed", nil
		}
		if err != nil {
			return "", err
		}
		result, err := controller.Wait(child, WaitRequest{TaskIDs: []string{next.TaskID}, EventDriven: true})
		if err != nil {
			return "", err
		}
		if len(result.Tasks) != 1 || result.Tasks[0].Status != StatusCompleted {
			return "", fmt.Errorf("descendant did not complete: %+v", result)
		}
		return result.Tasks[0].ResultContent, nil
	}
	m = NewManager(ctx, Config{Starter: starter})
	defer m.Close(context.Background())
	root, leave, err := m.EnterExecution(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer leave()
	first, err := m.Spawn(root, SpawnRequest{Description: "depth-1", Prompt: "1"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Wait(root, WaitRequest{TaskIDs: []string{first.TaskID}, EventDriven: true})
	if err != nil || len(result.Tasks) != 1 || result.Tasks[0].ResultContent != "depth-eight-confirmed" {
		t.Fatalf("recursive wait: %+v %v", result, err)
	}
	if starts.Load() != 8 || len(m.List()) != 8 {
		t.Fatalf("depth boundary: starts=%d tasks=%d", starts.Load(), len(m.List()))
	}
}
