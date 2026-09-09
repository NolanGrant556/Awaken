package filestore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLockWaitsForInFlightOwnerInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, FilePerm)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		time.Sleep(20 * time.Millisecond)
		encoded, _ := json.Marshal(lockOwner{PID: os.Getpid(), OwnerID: "first", ExpiresAt: time.Now().Add(time.Minute)})
		_, writeErr := file.Write(encoded)
		done <- errors.Join(writeErr, file.Close())
	}()
	lock, acquireErr := AcquireLock(path, "second", time.Minute)
	if lock != nil {
		_ = lock.Release()
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(acquireErr, ErrLockHeld) {
		t.Fatalf("in-flight owner should be recognized after initialization: %v", acquireErr)
	}
	data, err := os.ReadFile(path)
	var owner lockOwner
	if err != nil || json.Unmarshal(data, &owner) != nil || owner.OwnerID != "first" {
		t.Fatalf("original owner changed: %s %v", data, err)
	}
}
