package skillruntime

import (
	"context"
	"testing"

	"awaken/src/skills"
)

type argumentBodyFetcher struct {
	calls int
	args  map[string]string
}

func (f *argumentBodyFetcher) FetchBody(_ context.Context, _ skills.McpBinding, args map[string]string) (string, error) {
	f.calls++
	f.args = args
	return "server-rendered ${name}", nil
}

func TestCatalogInvocationPassesNamedMCPArguments(t *testing.T) {
	fetcher := &argumentBodyFetcher{}
	registry := skills.NewSkillRegistry(nil)
	registry.RegisterSkill(&skills.SkillDefinition{ID: "mcp__test__prompt", Name: "prompt", State: skills.StateLoaded, Source: skills.SourceMcp,
		Arguments:  []skills.SkillArgument{{Name: "name", Required: true}},
		McpBinding: &skills.McpBinding{Kind: "prompt", ServerName: "test", PromptName: "prompt"}, BodyFetcher: fetcher})
	tool := &CatalogTool{Registry: registry, Activator: NewActivator(registry, nil)}
	result, err := tool.Invoke(context.Background(), SkillInvocationRequest{Skill: "mcp__test__prompt", Args: `{"name":"Ada"}`})
	if err != nil || fetcher.calls != 1 || fetcher.args["name"] != "Ada" {
		t.Fatalf("catalog arguments lost: %+v %v %#v", result, err, fetcher.args)
	}
	if result.Plan.MainBody != "server-rendered ${name}" {
		t.Fatalf("server body was rendered again: %q", result.Plan.MainBody)
	}
	for _, raw := range []string{"", "name=Ada", `{"name":3}`} {
		if _, err := tool.Invoke(context.Background(), SkillInvocationRequest{Skill: "mcp__test__prompt", Args: raw}); err == nil {
			t.Fatalf("invalid arguments accepted: %s", raw)
		}
	}
	if fetcher.calls != 1 {
		t.Fatal("invalid arguments reached the server")
	}
}
