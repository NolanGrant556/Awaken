package verifierinit

import (
	"reflect"
	"strings"
	"testing"
)

func TestSingleWebAppRecommendsVerifierPlaywright(t *testing.T) {
	files := []ProjectFile{
		{
			Path: "package.json",
			Content: `{
				"name": "awaken-ui",
				"scripts": { "dev": "vite --host 127.0.0.1" },
				"dependencies": { "react": "latest", "vite": "latest" }
			}`,
		},
	}

	areas := DetectAreas(files)
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	if areas[0].Kind != AreaWebApp {
		t.Fatalf("expected web app area, got %q", areas[0].Kind)
	}

	plans := RecommendVerifiers(areas, ToolState{})
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan, got %d", len(plans))
	}
	if plans[0].Name != "verifier-playwright" {
		t.Fatalf("expected verifier-playwright, got %q", plans[0].Name)
	}
	if plans[0].Type != VerifierPlaywright {
		t.Fatalf("expected playwright plan, got %q", plans[0].Type)
	}
}

func TestMultipleFrontendBackendNames(t *testing.T) {
	files := []ProjectFile{
		{
			Path: "frontend/package.json",
			Content: `{
				"name": "@awaken/web",
				"scripts": { "dev": "vite" },
				"dependencies": { "react": "latest", "vite": "latest" }
			}`,
		},
		{
			Path: "backend/package.json",
			Content: `{
				"name": "@awaken/service",
				"scripts": { "dev": "node server.js" },
				"dependencies": { "express": "latest" }
			}`,
		},
	}

	plans := RecommendVerifiers(DetectAreas(files), ToolState{})
	got := planNames(plans)
	want := []string{"verifier-backend-api", "verifier-frontend-playwright"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected plan names %v, got %v", want, got)
	}
}

func TestCLIAndAPIDetection(t *testing.T) {
	files := []ProjectFile{
		{
			Path: "tools/package.json",
			Content: `{
				"name": "awaken-cli",
				"bin": { "awaken": "./bin/awaken.js" },
				"dependencies": { "commander": "latest" }
			}`,
		},
		{
			Path: "api/pyproject.toml",
			Content: `[project]
name = "awaken-api"
dependencies = ["fastapi", "uvicorn"]
`,
		},
	}

	areas := DetectAreas(files)
	kinds := map[string]AreaKind{}
	for _, area := range areas {
		kinds[area.Path] = area.Kind
	}
	if kinds["tools"] != AreaCLI {
		t.Fatalf("expected tools to be CLI, got %q", kinds["tools"])
	}
	if kinds["api"] != AreaAPI {
		t.Fatalf("expected api to be API, got %q", kinds["api"])
	}

	plans := RecommendVerifiers(areas, ToolState{})
	got := planNames(plans)
	want := []string{"verifier-api-api", "verifier-tools-cli"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected plan names %v, got %v", want, got)
	}
}

func TestSharedLibDoesNotCreateVerifier(t *testing.T) {
	files := []ProjectFile{
		{
			Path: "packages/shared/package.json",
			Content: `{
				"name": "@awaken/shared",
				"main": "./dist/index.js",
				"devDependencies": { "vitest": "latest", "typescript": "latest" }
			}`,
		},
	}

	areas := DetectAreas(files)
	if len(areas) != 1 {
		t.Fatalf("expected 1 area, got %d", len(areas))
	}
	if areas[0].Kind != AreaSharedLib {
		t.Fatalf("expected shared lib, got %q", areas[0].Kind)
	}
	if plans := RecommendVerifiers(areas, ToolState{}); len(plans) != 0 {
		t.Fatalf("expected no verifier plans, got %v", planNames(plans))
	}
}

func TestSkillPathContainsClaudeSkillsAndVerifier(t *testing.T) {
	got := SkillPath(VerifierPlan{Name: "playwright"})
	if !strings.Contains(got, ".claude/skills/") {
		t.Fatalf("expected path to contain .claude/skills, got %q", got)
	}
	if !strings.Contains(got, "verifier") {
		t.Fatalf("expected path to contain verifier, got %q", got)
	}
	if !strings.HasSuffix(got, "/SKILL.md") {
		t.Fatalf("expected SKILL.md path, got %q", got)
	}
}

func TestRenderedSkillIncludesAllowedToolsAuthEnvAndSelfUpdate(t *testing.T) {
	plan := VerifierPlan{
		Name: "verifier-playwright",
		Type: VerifierPlaywright,
		Area: Area{
			Name:           "frontend",
			Path:           "frontend",
			Kind:           AreaWebApp,
			PackageManager: "pnpm",
			DevCommand:     "pnpm run dev",
			BaseURL:        "http://localhost:5173",
			ReadySignal:    "Local:",
		},
		AllowedTools: allowedToolsFor(VerifierPlaywright, ToolState{PlaywrightMCP: true}),
		Auth: AuthRequirement{
			Required:       true,
			Method:         "form login",
			LoginURL:       "/login",
			EnvVars:        []string{"TEST_USER", "TEST_PASSWORD"},
			PostLoginCheck: "dashboard appears",
		},
	}

	rendered := RenderSkill(plan)
	assertContains(t, rendered, "allowed-tools:")
	assertContains(t, rendered, "mcp__playwright__*")
	assertContains(t, rendered, "$TEST_USER")
	assertContains(t, rendered, "$TEST_PASSWORD")
	assertContains(t, rendered, "Self-Update")
	assertContains(t, rendered, "AskUserQuestion")
	assertContains(t, rendered, "Edit this SKILL.md")
}

func TestRenderedSkillIncludesPlaywrightInstallCommandsForPackageManagers(t *testing.T) {
	rendered := RenderSkill(VerifierPlan{
		Name: "verifier-playwright",
		Type: VerifierPlaywright,
		Area: Area{Path: ".", Kind: AreaWebApp, PackageManager: "npm"},
	})

	for _, command := range []string{
		"npm install -D @playwright/test && npx playwright install",
		"yarn add -D @playwright/test && yarn playwright install",
		"pnpm add -D @playwright/test && pnpm exec playwright install",
		"bun add -D @playwright/test && bun playwright install",
	} {
		assertContains(t, rendered, command)
	}
}

func TestRenderedSkillDoesNotIncludeHardcodedCredentialValues(t *testing.T) {
	rendered := RenderSkill(VerifierPlan{
		Name: "verifier-api",
		Type: VerifierAPI,
		Area: Area{Path: "api", Kind: AreaAPI},
		Auth: AuthRequirement{
			Required: true,
			Method:   "token password=hunter2",
			LoginURL: "http://localhost:3000/login?token=sk-live-secret",
			EnvVars:  []string{"TEST_API_TOKEN"},
		},
	})

	for _, secret := range []string{"hunter2", "sk-live-secret", "password=hunter2", "token=sk-live-secret"} {
		if strings.Contains(rendered, secret) {
			t.Fatalf("rendered skill leaked hardcoded credential value %q:\n%s", secret, rendered)
		}
	}
	assertContains(t, rendered, "$TEST_API_TOKEN")
}

func planNames(plans []VerifierPlan) []string {
	names := make([]string, 0, len(plans))
	for _, plan := range plans {
		names = append(names, plan.Name)
	}
	return names
}

func assertContains(t *testing.T, text, substring string) {
	t.Helper()
	if !strings.Contains(text, substring) {
		t.Fatalf("expected text to contain %q:\n%s", substring, text)
	}
}
