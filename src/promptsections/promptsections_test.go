package promptsections

import (
	"context"
	"errors"
	"testing"
)

func TestCacheableSectionComputesOnce(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()
	calls := 0

	section := cache.Section("identity", func(context.Context) (string, bool, error) {
		calls++
		return "stable identity", true, nil
	})

	for i := 0; i < 3; i++ {
		resolved, err := cache.Resolve(ctx, []Section{section})
		if err != nil {
			t.Fatalf("expected resolve to succeed: %v", err)
		}
		if len(resolved) != 1 {
			t.Fatalf("expected one resolved section, got %d", len(resolved))
		}
		if resolved[0].Content != "stable identity" {
			t.Fatalf("expected cached content, got %q", resolved[0].Content)
		}
		if !resolved[0].Present {
			t.Fatal("expected section to be present")
		}
	}

	if calls != 1 {
		t.Fatalf("expected compute to run once, got %d", calls)
	}
}

func TestUncachedSectionComputesEveryTime(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()
	calls := 0

	section := cache.UncachedSection("clock", "clock depends on current time", func(context.Context) (string, bool, error) {
		calls++
		return "tick", true, nil
	})

	for i := 0; i < 3; i++ {
		resolved, err := cache.Resolve(ctx, []Section{section})
		if err != nil {
			t.Fatalf("expected resolve to succeed: %v", err)
		}
		if len(resolved) != 1 || resolved[0].Content != "tick" {
			t.Fatalf("unexpected resolved section: %#v", resolved)
		}
	}

	if calls != 3 {
		t.Fatalf("expected compute to run every time, got %d calls", calls)
	}
}

func TestClearForcesCacheableSectionToRecompute(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()
	calls := 0

	section := cache.Section("runtime", func(context.Context) (string, bool, error) {
		calls++
		return "value", true, nil
	})

	if _, err := cache.Resolve(ctx, []Section{section}); err != nil {
		t.Fatalf("expected first resolve to succeed: %v", err)
	}
	if _, err := cache.Resolve(ctx, []Section{section}); err != nil {
		t.Fatalf("expected cached resolve to succeed: %v", err)
	}

	cache.Clear()

	if _, err := cache.Resolve(ctx, []Section{section}); err != nil {
		t.Fatalf("expected resolve after clear to succeed: %v", err)
	}

	if calls != 2 {
		t.Fatalf("expected clear to force one recompute, got %d calls", calls)
	}
}

func TestResolvePreservesOrderAndPresence(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()

	sections := []Section{
		cache.Section("first", func(context.Context) (string, bool, error) {
			return "one", true, nil
		}),
		{},
		cache.Section("empty", func(context.Context) (string, bool, error) {
			return "", true, nil
		}),
		cache.Section("absent", func(context.Context) (string, bool, error) {
			return "", false, nil
		}),
		cache.UncachedSection("last", "must be rebuilt for each request", func(context.Context) (string, bool, error) {
			return "four", true, nil
		}),
	}

	resolved, err := cache.Resolve(ctx, sections)
	if err != nil {
		t.Fatalf("expected resolve to succeed: %v", err)
	}

	if len(resolved) != len(sections) {
		t.Fatalf("expected %d resolved sections, got %d", len(sections), len(resolved))
	}

	expectedNames := []string{"first", "", "empty", "absent", "last"}
	expectedPresence := []bool{true, false, true, false, true}
	for i := range resolved {
		if resolved[i].Name != expectedNames[i] {
			t.Fatalf("section %d name: expected %q, got %q", i, expectedNames[i], resolved[i].Name)
		}
		if resolved[i].Present != expectedPresence[i] {
			t.Fatalf("section %d presence: expected %v, got %v", i, expectedPresence[i], resolved[i].Present)
		}
	}

	if resolved[2].Content != "" {
		t.Fatalf("expected empty section content to stay empty, got %q", resolved[2].Content)
	}
	if !resolved[4].CacheBreak || resolved[4].Reason == "" {
		t.Fatalf("expected uncached metadata to be preserved: %#v", resolved[4])
	}
}

func TestErrorDoesNotPolluteCache(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()
	calls := 0
	sentinel := errors.New("boom")

	section := cache.Section("flaky", func(context.Context) (string, bool, error) {
		calls++
		if calls == 1 {
			return "", false, sentinel
		}
		return "ok", true, nil
	})

	if _, err := cache.Resolve(ctx, []Section{section}); !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	resolved, err := cache.Resolve(ctx, []Section{section})
	if err != nil {
		t.Fatalf("expected second resolve to succeed: %v", err)
	}
	if resolved[0].Content != "ok" || !resolved[0].Present {
		t.Fatalf("unexpected resolved section after retry: %#v", resolved[0])
	}

	if _, err := cache.Resolve(ctx, []Section{section}); err != nil {
		t.Fatalf("expected cached retry to succeed: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected successful retry to populate cache, got %d calls", calls)
	}
}

func TestUncachedSectionRequiresReason(t *testing.T) {
	ctx := context.Background()
	cache := NewCache()
	calls := 0

	section := cache.UncachedSection("danger", "", func(context.Context) (string, bool, error) {
		calls++
		return "uncached", true, nil
	})

	_, err := cache.Resolve(ctx, []Section{section})
	if !errors.Is(err, ErrMissingUncachedReason) {
		t.Fatalf("expected missing reason error, got %v", err)
	}
	if calls != 0 {
		t.Fatalf("expected invalid uncached section not to compute, got %d calls", calls)
	}
}
