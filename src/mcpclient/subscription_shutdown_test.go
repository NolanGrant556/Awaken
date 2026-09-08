package mcpclient

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestSubscriberCancellationAndShutdownHaveSingleCloseOwner(t *testing.T) {
	for i := 0; i < 100; i++ {
		manager := &Manager{}
		ctx, cancel := context.WithCancel(context.Background())
		ch := manager.Subscribe(ctx)
		var wg sync.WaitGroup
		wg.Add(3)
		go func() { defer wg.Done(); cancel() }()
		go func() { defer wg.Done(); manager.Shutdown(context.Background()) }()
		go func() { defer wg.Done(); manager.emitEvent(ServerEvent{}) }()
		wg.Wait()
		manager.Shutdown(context.Background())
		for range ch {
		}
		late := manager.Subscribe(context.Background())
		select {
		case _, open := <-late:
			if open {
				t.Fatal("subscription after shutdown remained open")
			}
		case <-time.After(time.Second):
			t.Fatal("subscription after shutdown blocked")
		}
	}
}
