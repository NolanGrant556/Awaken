package main

import (
	"context"
	"errors"
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
	if time.Since(start) > time.Second {
		t.Fatal("owner ignored total deadline")
	}
}
