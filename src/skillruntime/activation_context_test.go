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

func TestSkillToolActivationScopeIsConcurrentAndImmutable(t *testing.T) {
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
	st := &SkillTool{
		SkillID:   "scoped",
		NameStr:   SkillToolName("scoped"),
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
			if _, err := st.Execute(ctx, `{}`); err != nil {
				t.Errorf("execute parent=%v: %v", parent, err)
			}
		}()
	}
	wg.Wait()
	close(observed)

	var observedPools []string
	for pool := range observed {
		observedPools = append(observedPools, pool)
	}
	sort.Strings(observedPools)
	mu.Lock()
	sort.Strings(executorPools)
	gotExecutorPools := append([]string(nil), executorPools...)
	mu.Unlock()
	if strings.Join(observedPools, "|") != "alpha|beta" {
		t.Fatalf("observed plans leaked: %v", observedPools)
	}
	if strings.Join(gotExecutorPools, "|") != "alpha|beta" {
		t.Fatalf("observer mutated executor plans: %v", gotExecutorPools)
	}
}

func TestSkillToolActivationObserverFailurePreventsExecution(t *testing.T) {
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
	st := &SkillTool{
		SkillID:   "durable",
		NameStr:   SkillToolName("durable"),
		Activator: NewActivator(reg, nil),
		Executor: SkillExecutorFunc(func(context.Context, string, string, []string) (string, error) {
			executed = true
			return "ok", nil
		}),
	}
	ctx := WithActivationScope(context.Background(), []string{"file_read"}, func(context.Context, *SkillActivationPlan) error {
		return context.Canceled
	})
	if _, err := st.Execute(ctx, `{}`); err == nil {
		t.Fatal("expected observer persistence failure")
	}
	if executed {
		t.Fatal("skill executor ran before activation was durably accepted")
	}
}
