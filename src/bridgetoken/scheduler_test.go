package bridgetoken

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSchedulerSchedulesFromJWTExp(t *testing.T) {
	now := time.Unix(1700000000, 0)
	clock := newFakeClock(now)
	refreshes := make(chan string, 1)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    5 * time.Minute,
		FallbackInterval: time.Hour,
		GetAccessToken: func() (string, error) {
			return "oauth-token", nil
		},
		OnRefresh: func(sessionID string, token string) {
			refreshes <- sessionID + ":" + token
		},
	})

	token := testJWT(t, map[string]any{"exp": now.Add(10 * time.Minute).Unix()})
	if !scheduler.Schedule("session-1", token) {
		t.Fatal("expected JWT schedule to succeed")
	}

	clock.Advance(5*time.Minute - time.Nanosecond)
	assertNoString(t, refreshes, "refresh before JWT exp buffer")

	clock.Advance(time.Nanosecond)
	assertString(t, refreshes, "session-1:oauth-token")
}

func TestSchedulerSchedulesFromExpiresIn(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	refreshes := make(chan string, 1)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    30 * time.Second,
		FallbackInterval: time.Hour,
		GetAccessToken: func() (string, error) {
			return "oauth-token", nil
		},
		OnRefresh: func(sessionID string, token string) {
			refreshes <- sessionID + ":" + token
		},
	})

	if !scheduler.ScheduleFromExpiresIn("session-1", 120) {
		t.Fatal("expected expires_in schedule to succeed")
	}

	clock.Advance(90*time.Second - time.Nanosecond)
	assertNoString(t, refreshes, "refresh before expires_in buffer")

	clock.Advance(time.Nanosecond)
	assertString(t, refreshes, "session-1:oauth-token")
}

func TestSchedulerImmediateRefresh(t *testing.T) {
	now := time.Unix(1700000000, 0)
	clock := newFakeClock(now)
	refreshes := make(chan string, 1)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    time.Minute,
		FallbackInterval: time.Hour,
		GetAccessToken: func() (string, error) {
			return "oauth-token", nil
		},
		OnRefresh: func(sessionID string, token string) {
			refreshes <- sessionID + ":" + token
		},
	})

	token := testJWT(t, map[string]any{"exp": now.Add(30 * time.Second).Unix()})
	if !scheduler.Schedule("session-1", token) {
		t.Fatal("expected immediate JWT schedule to succeed")
	}

	assertString(t, refreshes, "session-1:oauth-token")
}

func TestSchedulerRetriesUntilCap(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	attempts := make(chan int, 4)
	var count atomic.Int32
	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    time.Second,
		RetryDelay:       10 * time.Second,
		FallbackInterval: time.Hour,
		MaxFailures:      3,
		GetAccessToken: func() (string, error) {
			next := int(count.Add(1))
			attempts <- next
			return "", nil
		},
	})

	scheduler.ScheduleFromExpiresIn("session-1", 0)
	assertInt(t, attempts, 1)

	clock.Advance(10 * time.Second)
	assertInt(t, attempts, 2)

	clock.Advance(10 * time.Second)
	assertInt(t, attempts, 3)

	clock.Advance(10 * time.Second)
	assertNoInt(t, attempts, "attempt after retry cap")
}

func TestSchedulerSchedulesFallbackAfterSuccessfulRefresh(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	refreshes := make(chan string, 2)
	var count atomic.Int32
	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    time.Second,
		FallbackInterval: 10 * time.Second,
		GetAccessToken: func() (string, error) {
			next := count.Add(1)
			return "oauth-token-" + strconv.Itoa(int(next)), nil
		},
		OnRefresh: func(sessionID string, token string) {
			refreshes <- sessionID + ":" + token
		},
	})

	scheduler.ScheduleFromExpiresIn("session-1", 0)
	assertString(t, refreshes, "session-1:oauth-token-1")

	clock.Advance(10 * time.Second)
	assertString(t, refreshes, "session-1:oauth-token-2")
}

func TestSchedulerSkipsStaleRefreshAfterReschedule(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	started := make(chan struct{})
	release := make(chan struct{})
	firstReturned := make(chan struct{})
	refreshes := make(chan string, 2)
	var count atomic.Int32
	var closeStarted sync.Once

	scheduler := NewScheduler(SchedulerConfig{
		Clock:            clock,
		RefreshBuffer:    time.Second,
		FallbackInterval: time.Hour,
		GetAccessToken: func() (string, error) {
			next := count.Add(1)
			if next == 1 {
				closeStarted.Do(func() { close(started) })
				<-release
				close(firstReturned)
				return "stale-token", nil
			}
			return "fresh-token", nil
		},
		OnRefresh: func(sessionID string, token string) {
			refreshes <- sessionID + ":" + token
		},
	})

	scheduler.ScheduleFromExpiresIn("session-1", 0)
	<-started

	scheduler.ScheduleFromExpiresIn("session-1", 10)
	close(release)
	<-firstReturned
	assertNoString(t, refreshes, "stale refresh")

	clock.Advance(9 * time.Second)
	assertString(t, refreshes, "session-1:fresh-token")
}

func TestSchedulerCancelStopsTimer(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	attempts := make(chan int, 1)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:         clock,
		RefreshBuffer: time.Second,
		GetAccessToken: func() (string, error) {
			attempts <- 1
			return "oauth-token", nil
		},
	})

	scheduler.ScheduleFromExpiresIn("session-1", 10)
	scheduler.Cancel("session-1")
	clock.Advance(9 * time.Second)

	assertNoInt(t, attempts, "refresh after cancel")
}

func TestSchedulerCancelAllStopsTimers(t *testing.T) {
	clock := newFakeClock(time.Unix(1700000000, 0))
	attempts := make(chan int, 2)
	scheduler := NewScheduler(SchedulerConfig{
		Clock:         clock,
		RefreshBuffer: time.Second,
		GetAccessToken: func() (string, error) {
			attempts <- 1
			return "oauth-token", nil
		},
	})

	scheduler.ScheduleFromExpiresIn("session-1", 10)
	scheduler.ScheduleFromExpiresIn("session-2", 10)
	scheduler.CancelAll()
	clock.Advance(9 * time.Second)

	assertNoInt(t, attempts, "refresh after cancel all")
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

type fakeTimer struct {
	clock   *fakeClock
	at      time.Time
	fn      func()
	stopped bool
	fired   bool
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(delay time.Duration, fn func()) Timer {
	if delay < 0 {
		delay = 0
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &fakeTimer{
		clock: c,
		at:    c.now.Add(delay),
		fn:    fn,
	}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *fakeClock) Advance(delay time.Duration) {
	target := c.Now().Add(delay)
	for {
		timer := c.nextDueTimer(target)
		if timer == nil {
			c.mu.Lock()
			c.now = target
			c.mu.Unlock()
			return
		}
		timer.fn()
	}
}

func (c *fakeClock) nextDueTimer(target time.Time) *fakeTimer {
	c.mu.Lock()
	defer c.mu.Unlock()

	var next *fakeTimer
	for _, timer := range c.timers {
		if timer.stopped || timer.fired || timer.at.After(target) {
			continue
		}
		if next == nil || timer.at.Before(next.at) {
			next = timer
		}
	}
	if next == nil {
		return nil
	}
	c.now = next.at
	next.fired = true
	return next
}

func (t *fakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

func assertString(t *testing.T, ch <-chan string, expected string) {
	t.Helper()
	select {
	case actual := <-ch:
		if actual != expected {
			t.Fatalf("expected %q, got %q", expected, actual)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for %q", expected)
	}
}

func assertNoString(t *testing.T, ch <-chan string, reason string) {
	t.Helper()
	select {
	case actual := <-ch:
		t.Fatalf("unexpected %s: %q", reason, actual)
	case <-time.After(20 * time.Millisecond):
	}
}

func assertInt(t *testing.T, ch <-chan int, expected int) {
	t.Helper()
	select {
	case actual := <-ch:
		if actual != expected {
			t.Fatalf("expected %d, got %d", expected, actual)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for %d", expected)
	}
}

func assertNoInt(t *testing.T, ch <-chan int, reason string) {
	t.Helper()
	select {
	case actual := <-ch:
		t.Fatalf("unexpected %s: %d", reason, actual)
	case <-time.After(20 * time.Millisecond):
	}
}
