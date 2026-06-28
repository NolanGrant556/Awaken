package verifierinit

import (
	"path/filepath"
	"strings"
)

func SkillPath(plan VerifierPlan) string {
	name := ensureVerifierName(slug(plan.Name))
	if name == "" {
		name = "verifier"
	}
	return filepath.ToSlash(filepath.Join(".claude", "skills", name, "SKILL.md"))
}

func RenderSkill(plan VerifierPlan) string {
	name := ensureVerifierName(slug(plan.Name))
	if name == "" {
		name = "verifier"
	}
	description := plan.Description
	if description == "" {
		description = defaultDescription(plan)
	}
	allowedTools := plan.AllowedTools
	if len(allowedTools) == 0 {
		allowedTools = allowedToolsFor(plan.Type, ToolState{})
	}

	var builder strings.Builder
	builder.WriteString("---\n")
	builder.WriteString("name: " + name + "\n")
	builder.WriteString("description: " + description + "\n")
	builder.WriteString("allowed-tools:\n")
	for _, tool := range allowedTools {
		builder.WriteString("  - " + tool + "\n")
	}
	builder.WriteString("---\n\n")

	builder.WriteString("# " + titleFor(plan, name) + "\n\n")
	builder.WriteString("You are a verification executor. You receive a verification plan and execute it EXACTLY as written.\n\n")
	writeProjectContext(&builder, plan)
	writeSetupInstructions(&builder, plan)
	writeAuthentication(&builder, plan)
	writeReportingCleanup(&builder)
	writeSelfUpdate(&builder)
	return builder.String()
}

func writeProjectContext(builder *strings.Builder, plan VerifierPlan) {
	area := plan.Area
	builder.WriteString("## Project Context\n\n")
	builder.WriteString("- Area: " + areaLabel(area) + "\n")
	if area.Path != "" {
		builder.WriteString("- Path: " + area.Path + "\n")
	}
	if area.Manifest != "" {
		builder.WriteString("- Manifest: " + area.Manifest + "\n")
	}
	if area.Kind != "" {
		builder.WriteString("- Detected type: " + string(area.Kind) + "\n")
	}
	if area.PackageManager != "" {
		builder.WriteString("- Package manager: " + area.PackageManager + "\n")
	}
	if len(area.Stack) > 0 {
		builder.WriteString("- Stack: " + strings.Join(area.Stack, ", ") + "\n")
	}
	if len(plan.BrowserOptions) > 0 {
		builder.WriteString("- Browser verification options: " + strings.Join(plan.BrowserOptions, ", ") + "\n")
	}
	builder.WriteString("\nDo not turn unit tests or typechecking into verifier steps; those belong in the normal build/test workflow.\n\n")
}

func writeSetupInstructions(builder *strings.Builder, plan VerifierPlan) {
	builder.WriteString("## Setup Instructions\n\n")
	switch plan.Type {
	case VerifierPlaywright:
		command := plan.Area.DevCommand
		if command == "" {
			command = packageRunCommand(defaultPackageManager(plan.Area.PackageManager), "dev")
		}
		baseURL := plan.Area.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:3000"
		}
		ready := plan.Area.ReadySignal
		if ready == "" {
			ready = "ready"
		}
		builder.WriteString("1. Start the web app with `" + command + "` from `" + plan.Area.Path + "`.\n")
		builder.WriteString("2. Wait until the server reports `" + ready + "`.\n")
		builder.WriteString("3. Open `" + baseURL + "` with Playwright or an approved browser MCP tool.\n")
		builder.WriteString("4. Verify the user-facing behavior from the verification plan with real browser interactions.\n\n")
		builder.WriteString("If Playwright is missing, use the package-manager command that matches this project:\n\n")
		for _, command := range playwrightInstallCommands() {
			builder.WriteString("- " + command + "\n")
		}
		builder.WriteString("\n")
	case VerifierAPI:
		command := plan.Area.DevCommand
		if command == "" {
			command = "start the API server with the project command"
		}
		baseURL := plan.Area.BaseURL
		if baseURL == "" {
			baseURL = "http://localhost:3000"
		}
		builder.WriteString("1. Start the API service with `" + command + "` from `" + plan.Area.Path + "`.\n")
		builder.WriteString("2. Use `" + baseURL + "` as the base URL unless the verification plan overrides it.\n")
		builder.WriteString("3. Exercise endpoints with `curl` or `http` and verify response status, headers, and body.\n\n")
	case VerifierCLI:
		command := plan.Area.EntryCommand
		if command == "" {
			command = "start the CLI entry point"
		}
		builder.WriteString("1. Start a clean Tmux session for the verification.\n")
		builder.WriteString("2. Run `" + command + "` and interact with the terminal exactly as the plan requires.\n")
		builder.WriteString("3. Use asciinema only when a recording is requested or useful for evidence.\n\n")
	default:
		builder.WriteString("Follow the verification plan for this project area.\n\n")
	}
}

func writeAuthentication(builder *strings.Builder, plan VerifierPlan) {
	auth := plan.Auth
	builder.WriteString("## Authentication\n\n")
	if auth.Required {
		envVars := auth.EnvVars
		if len(envVars) == 0 {
			envVars = defaultAuthEnvVars(plan.Type)
		}
		builder.WriteString("This verifier requires authentication. Use environment variables only; never hardcode usernames, passwords, API keys, tokens, or secret values in this skill or in commands.\n\n")
		if auth.Method != "" {
			builder.WriteString("- Method: " + safeText(auth.Method) + "\n")
		}
		if auth.LoginURL != "" {
			builder.WriteString("- Login URL: " + safeText(auth.LoginURL) + "\n")
		}
		builder.WriteString("- Required env vars: ")
		for index, envVar := range envVars {
			if index > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("$" + safeEnvName(envVar))
		}
		builder.WriteString("\n")
		if auth.PostLoginCheck != "" {
			builder.WriteString("- Post-login check: " + safeText(auth.PostLoginCheck) + "\n")
		}
		builder.WriteString("\n")
		return
	}

	builder.WriteString("No authentication requirement was detected. If a plan needs login or API credentials, add named environment variables such as `$TEST_USER`, `$TEST_PASSWORD`, or `$TEST_API_TOKEN`; do not write credential values into this file.\n\n")
}

func writeReportingCleanup(builder *strings.Builder) {
	builder.WriteString("## Reporting\n\n")
	builder.WriteString("Report PASS or FAIL for each step using the format specified in the verification plan. Include concise evidence for failures.\n\n")
	builder.WriteString("## Cleanup\n\n")
	builder.WriteString("After verification:\n")
	builder.WriteString("1. Stop any dev servers started\n")
	builder.WriteString("2. Close any browser, API, Tmux, or recording sessions opened for this run\n")
	builder.WriteString("3. Report final summary\n\n")
}

func writeSelfUpdate(builder *strings.Builder) {
	builder.WriteString("## Self-Update\n\n")
	builder.WriteString("If verification fails because this skill's instructions are outdated, such as a changed dev server command, port, ready signal, entry command, base URL, or authentication flow, and not because the feature under test is broken, use AskUserQuestion to confirm and then Edit this SKILL.md with a minimal targeted fix.\n")
}

func titleFor(plan VerifierPlan, name string) string {
	switch plan.Type {
	case VerifierPlaywright:
		return name + " Browser Verifier"
	case VerifierAPI:
		return name + " API Verifier"
	case VerifierCLI:
		return name + " CLI Verifier"
	default:
		return name
	}
}

func defaultDescription(plan VerifierPlan) string {
	switch plan.Type {
	case VerifierPlaywright:
		return "Functional browser verification for " + areaLabel(plan.Area)
	case VerifierAPI:
		return "Functional HTTP API verification for " + areaLabel(plan.Area)
	case VerifierCLI:
		return "Functional terminal verification for " + areaLabel(plan.Area)
	default:
		return "Functional verification for " + areaLabel(plan.Area)
	}
}

func defaultPackageManager(pm string) string {
	if pm == "" {
		return "npm"
	}
	return pm
}

func playwrightInstallCommands() []string {
	return []string{
		"npm: `npm install -D @playwright/test && npx playwright install`",
		"yarn: `yarn add -D @playwright/test && yarn playwright install`",
		"pnpm: `pnpm add -D @playwright/test && pnpm exec playwright install`",
		"bun: `bun add -D @playwright/test && bun playwright install`",
	}
}

func defaultAuthEnvVars(verifierType VerifierType) []string {
	switch verifierType {
	case VerifierAPI:
		return []string{"TEST_API_TOKEN"}
	default:
		return []string{"TEST_USER", "TEST_PASSWORD"}
	}
}

func safeEnvName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "TEST_SECRET"
	}
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r - 32)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return strings.Trim(builder.String(), "_")
}

func safeText(value string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{"password", "token", "secret", "api_key", "api-key", "apikey", "authorization"} {
		for _, separator := range []string{"=", ":"} {
			if strings.Contains(lower, marker+separator) || strings.Contains(lower, marker+" "+separator) {
				return "use environment variables for secret-bearing values"
			}
		}
	}
	return value
}
