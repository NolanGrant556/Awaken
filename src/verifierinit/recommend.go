package verifierinit

import (
	"fmt"
	"sort"
	"strings"
)

func RecommendVerifiers(areas []Area, tools ToolState) []VerifierPlan {
	actionable := make([]Area, 0, len(areas))
	for _, area := range areas {
		switch area.Kind {
		case AreaWebApp, AreaAPI, AreaCLI:
			actionable = append(actionable, area)
		}
	}

	plans := make([]VerifierPlan, 0, len(actionable))
	multi := len(actionable) > 1
	seenNames := map[string]int{}
	for _, area := range actionable {
		plan := planForArea(area, tools, multi)
		if plan.Name == "" {
			continue
		}
		plan.Name = uniqueName(plan.Name, seenNames)
		plans = append(plans, plan)
	}
	return plans
}

func planForArea(area Area, tools ToolState, multi bool) VerifierPlan {
	switch area.Kind {
	case AreaWebApp:
		name := "verifier-playwright"
		if multi {
			name = "verifier-" + areaSlug(area) + "-playwright"
		}
		return VerifierPlan{
			Name:           name,
			Type:           VerifierPlaywright,
			Area:           area,
			Description:    "Functional browser verification for " + areaLabel(area),
			AllowedTools:   allowedToolsFor(VerifierPlaywright, tools),
			BrowserOptions: browserOptions(tools),
		}
	case AreaAPI:
		name := "verifier-api"
		if multi {
			name = "verifier-" + areaSlug(area) + "-api"
		}
		return VerifierPlan{
			Name:         name,
			Type:         VerifierAPI,
			Area:         area,
			Description:  "Functional HTTP API verification for " + areaLabel(area),
			AllowedTools: allowedToolsFor(VerifierAPI, tools),
		}
	case AreaCLI:
		name := "verifier-cli"
		if multi {
			name = "verifier-" + areaSlug(area) + "-cli"
		}
		return VerifierPlan{
			Name:         name,
			Type:         VerifierCLI,
			Area:         area,
			Description:  "Functional terminal verification for " + areaLabel(area),
			AllowedTools: allowedToolsFor(VerifierCLI, tools),
		}
	default:
		return VerifierPlan{}
	}
}

func allowedToolsFor(verifierType VerifierType, tools ToolState) []string {
	switch verifierType {
	case VerifierPlaywright:
		allowed := []string{
			"Bash(npm:*)",
			"Bash(yarn:*)",
			"Bash(pnpm:*)",
			"Bash(bun:*)",
			"mcp__playwright__*",
		}
		if tools.ChromeDevToolsMCP {
			allowed = append(allowed, "mcp__chrome_devtools__*")
		}
		if tools.ClaudeChromeExtensionMCP {
			allowed = append(allowed, "mcp__browser_use__*")
		}
		return append(allowed, "Read", "Glob", "Grep")
	case VerifierAPI:
		return []string{
			"Bash(curl:*)",
			"Bash(http:*)",
			"Bash(npm:*)",
			"Bash(yarn:*)",
			"Read",
			"Glob",
			"Grep",
		}
	case VerifierCLI:
		return []string{
			"Tmux",
			"Bash(asciinema:*)",
			"Read",
			"Glob",
			"Grep",
		}
	default:
		return nil
	}
}

func browserOptions(tools ToolState) []string {
	options := []string{}
	if tools.PlaywrightPackage || tools.PlaywrightMCP {
		options = append(options, "Playwright")
	} else {
		options = append(options, "Playwright (recommended install)")
	}
	if tools.PlaywrightMCP {
		options = append(options, "Playwright MCP")
	} else {
		options = append(options, "Playwright MCP option")
	}
	if tools.ChromeDevToolsMCP {
		options = append(options, "Chrome DevTools MCP")
	}
	if tools.ClaudeChromeExtensionMCP {
		options = append(options, "Claude Chrome Extension MCP")
	}
	return options
}

func uniqueName(name string, seen map[string]int) string {
	name = ensureVerifierName(slug(name))
	seen[name]++
	if seen[name] == 1 {
		return name
	}
	return fmt.Sprintf("%s-%d", name, seen[name])
}

func ensureVerifierName(name string) string {
	if name == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(name), "verifier") {
		return name
	}
	return "verifier-" + name
}

func areaSlug(area Area) string {
	candidates := []string{}
	if area.Path != "" && area.Path != "." {
		candidates = append(candidates, area.Path)
	}
	candidates = append(candidates, area.Name)
	for _, candidate := range candidates {
		candidate = slug(candidate)
		if candidate != "" && candidate != "." && candidate != "project" {
			return candidate
		}
	}
	return "project"
}

func areaLabel(area Area) string {
	if area.Name != "" {
		return area.Name
	}
	if area.Path != "" {
		return area.Path
	}
	return "project"
}

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "@")
	value = strings.ReplaceAll(value, "/", "-")
	value = strings.ReplaceAll(value, "_", "-")
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			builder.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(builder.String(), "-")
}

func sortedStrings(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}
