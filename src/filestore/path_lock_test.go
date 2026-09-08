package filestore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPathLockAcrossProcessesAndCancellation(t *testing.T) {
	if target := os.Getenv("AWAKEN_PATH_LOCK_TEST_TARGET"); target != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
		defer cancel()
		if err := WithPathLock(ctx, target, func() error { t.Fatal("entered a lock held by parent process"); return nil }); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("lock error: %v", err)
		}
		return
	}
	target := filepath.Join(t.TempDir(), "file.txt")
	if err := WithPathLock(context.Background(), target, func() error {
		// A different path must remain available while this one is held.
		if err := WithPathLock(context.Background(), target+".other", func() error { return nil }); err != nil {
			return err
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestPathLockAcrossProcessesAndCancellation$")
		cmd.Env = append(os.Environ(), "AWAKEN_PATH_LOCK_TEST_TARGET="+target)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, output)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Closing the handle must release the lock even after a cancelled waiter.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WithPathLock(ctx, target, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestPathLockSymlinkIdentity(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	a, err := pathLockIdentity(filepath.Join(root, "missing", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := pathLockIdentity(filepath.Join(alias, "missing", "file.txt"))
	if err != nil || a != b {
		t.Fatalf("aliases have different identities: %q %q %v", a, b, err)
	}
}
