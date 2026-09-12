package runcontrol

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestFirstSourceAndTreePropagation(t *testing.T) {
	root, cancel := WithCancel(context.Background())
	child, childCancel := WithCancel(root)
	defer childCancel()
	SetSource(root, Shutdown)
	cancel()
	SetSource(child, Parent)
	SetSource(root, User)
	if FromContext(root) != Shutdown || FromContext(child) != Shutdown {
		t.Fatal("cleanup overwrote original source")
	}
}

func TestDeadlineAndUnknown(t *testing.T) {
	expired, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	child, stop := WithCancel(expired)
	defer stop()
	if FromContext(child) != Timeout {
		t.Fatal("deadline lost")
	}
	old, oldCancel := context.WithCancel(context.Background())
	oldCancel()
	if FromContext(old) != Unknown {
		t.Fatal("invented user source")
	}
}

func TestConcurrentStopSourcesAreImmutable(t *testing.T) {
	ctx, cancel := WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	for _, source := range []Source{User, Parent, Shutdown, Timeout} {
		wg.Add(1)
		go func(s Source) { defer wg.Done(); SetSource(ctx, s); _ = FromContext(ctx) }(source)
	}
	wg.Wait()
	first := FromContext(ctx)
	SetSource(ctx, Unknown)
	if FromContext(ctx) != first || first == Unknown {
		t.Fatal("source not fixed")
	}
}
