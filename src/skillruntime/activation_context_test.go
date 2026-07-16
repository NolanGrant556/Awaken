package skillruntime

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"awaken/src/skills"
)

func TestCatalogToolActivationScopeIsConcurrentAndImmutable(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "scoped")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte(`---
name: scoped
description: scoped activation
---
Body
`), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})

	var mu sync.Mutex
	var executorPools []string
	catalogTool := &CatalogTool{
		Registry:  reg,
		Activator: NewActivator(reg, nil),
		Executor: SkillExecutorFunc(func(_ context.Context, _ string, _ string, allowed []string) (string, error) {
			mu.Lock()
			executorPools = append(executorPools, strings.Join(allowed, ","))
			mu.Unlock()
			return "ok", nil
		}),
	}

	parents := [][]string{{"alpha"}, {"beta"}}
	observed := make(chan string, len(parents))
	returned := make(chan string, len(parents))
	var wg sync.WaitGroup
	for _, parent := range parents {
		parent := append([]string(nil), parent...)
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := WithActivationScope(context.Background(), parent, func(_ context.Context, plan *SkillActivationPlan) error {
				observed <- strings.Join(plan.EffectiveTools, ",")
				// The observer owns a clone. Mutating it must not affect the plan
				// used by the Skill executor or the other concurrent invocation.
				plan.EffectiveTools[0] = "observer_mutation"
				return nil
			})
			result, err := catalogTool.Invoke(ctx, SkillInvocationRequest{Skill: "scoped", Source: InvocationUser})
			if err != nil {
				t.Errorf("execute parent=%v: %v", parent, err)
				return
			}
			returned <- strings.Join(result.Plan.EffectiveTools, ",")
		}()
	}
	wg.Wait()
	close(observed)
	close(returned)

	var observedPools []string
	for pool := range observed {
		observedPools = append(observedPools, pool)
	}
	var returnedPools []string
	for pool := range returned {
		returnedPools = append(returnedPools, pool)
	}
	sort.Strings(observedPools)
	sort.Strings(returnedPools)
	mu.Lock()
	sort.Strings(executorPools)
	gotExecutorPools := append([]string(nil), executorPools...)
	mu.Unlock()
	if strings.Join(observedPools, "|") != "alpha|beta" {
		t.Fatalf("observed plans leaked: %v", observedPools)
	}
	if strings.Join(returnedPools, "|") != "alpha|beta" {
		t.Fatalf("observer mutated returned plans: %v", returnedPools)
	}
	if strings.Join(gotExecutorPools, "|") != "|" {
		t.Fatalf("executor received parent permission pool instead of Skill hints: %v", gotExecutorPools)
	}
}

func TestCatalogToolActivationObserverFailurePreventsExecution(t *testing.T) {
	root := t.TempDir()
	skillRoot := filepath.Join(root, "durable")
	if err := os.MkdirAll(skillRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillRoot, "SKILL.md"), []byte("---\nname: durable\ndescription: durable\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	reg := skills.NewSkillRegistry(nil)
	reg.LoadFromSources(map[skills.SourceOrigin]string{skills.SourceUser: root})
	executed := false
	catalogTool := &CatalogTool{
		Registry:  reg,
		Activator: NewActivator(reg, nil),
		Executor: SkillExecutorFunc(func(context.Context, string, string, []string) (string, error) {
			executed = true
			return "ok", nil
		}),
	}
	ctx := WithActivationScope(context.Background(), []string{"Read"}, func(context.Context, *SkillActivationPlan) error {
		return context.Canceled
	})
	if _, err := catalogTool.Invoke(ctx, SkillInvocationRequest{Skill: "durable", Source: InvocationUser}); err == nil {
		t.Fatal("expected observer persistence failure")
	}
	if executed {
		t.Fatal("skill executor ran before activation was durably accepted")
	}
}
