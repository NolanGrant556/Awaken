package tool

import (
	"context"
	"os"
	"sort"
	"strings"
	"unicode"
)

// MCPFindToolName is the builtin auxiliary tool that searches deferred MCP
// tool definitions and loads them on demand.
const MCPFindToolName = "mcp_find"

// MCPDisclosureEnvVar overrides the progressive-disclosure policy:
// "off"/"false"/"0" disables deferral, "always"/"true"/"1" always defers,
// "auto" or "auto:<percent>" defers only when the estimated MCP definition
// size exceeds <percent> of the model context window. Empty uses auto with
// DefaultMCPDeferralThresholdPercent.
const MCPDisclosureEnvVar = "AWAKEN_MCP_DEFERRED_TOOLS"

const (
	// DefaultMCPDeferralThresholdPercent is the auto-mode activation
	// threshold: MCP definitions must exceed this share of the context
	// window before deferral turns on. Small catalogs stay fully loaded.
	DefaultMCPDeferralThresholdPercent = 10
	// DefaultMCPCatalogBudgetRunes bounds the rendered deferred-tool catalog
	// that is embedded in the mcp_find tool description.
	DefaultMCPCatalogBudgetRunes = 8000
)

// MCPDisclosureMode is the resolved deferral policy mode.
type MCPDisclosureMode string

const (
	MCPDisclosureAuto   MCPDisclosureMode = "auto"
	MCPDisclosureAlways MCPDisclosureMode = "always"
	MCPDisclosureOff    MCPDisclosureMode = "off"
)

// MCPDisclosurePolicy is the per-process deferral policy. It is resolved once
// per request build from the environment; the run snapshot and context window
// keep the resulting decision stable inside one run.
type MCPDisclosurePolicy struct {
	Mode             MCPDisclosureMode
	ThresholdPercent int
}

// ResolveMCPDisclosurePolicy parses a raw policy value. Unknown values fall
// back to the default auto policy so a typo can never silently disable MCP
// tools entirely.
func ResolveMCPDisclosurePolicy(raw string) MCPDisclosurePolicy {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "auto":
		return MCPDisclosurePolicy{Mode: MCPDisclosureAuto, ThresholdPercent: DefaultMCPDeferralThresholdPercent}
	case "off", "false", "0", "disabled":
		return MCPDisclosurePolicy{Mode: MCPDisclosureOff}
	case "always", "true", "1", "on":
		return MCPDisclosurePolicy{Mode: MCPDisclosureAlways}
	}
	if percent, ok := strings.CutPrefix(value, "auto:"); ok {
		parsed := 0
		valid := percent != ""
		for _, r := range percent {
			if r < '0' || r > '9' {
				valid = false
				break
			}
			parsed = parsed*10 + int(r-'0')
			if parsed > 100 {
				parsed = 100
			}
		}
		if valid {
			switch parsed {
			case 0:
				return MCPDisclosurePolicy{Mode: MCPDisclosureAlways}
			case 100:
				return MCPDisclosurePolicy{Mode: MCPDisclosureOff}
			default:
				return MCPDisclosurePolicy{Mode: MCPDisclosureAuto, ThresholdPercent: parsed}
			}
		}
	}
	return MCPDisclosurePolicy{Mode: MCPDisclosureAuto, ThresholdPercent: DefaultMCPDeferralThresholdPercent}
}

// MCPDisclosurePolicyFromEnv resolves the policy from MCPDisclosureEnvVar.
func MCPDisclosurePolicyFromEnv() MCPDisclosurePolicy {
	return ResolveMCPDisclosurePolicy(os.Getenv(MCPDisclosureEnvVar))
}

// MCPCatalogTool is one deferred-capable MCP tool observed in the pinned run
// snapshot. Names stay exactly as registered (mcp__<server>__<tool>).
type MCPCatalogTool struct {
	Name        string
	RawName     string
	Server      string
	Description string
	Readiness   ProviderReadiness
	InputSchema ToolParametersSchema
}

// MCPCatalogServer groups snapshot tools by their MCP server identity.
// Plugin-injected servers keep their full plugin__<plugin>__<server> identity.
type MCPCatalogServer struct {
	Server    string
	Readiness ProviderReadiness
	Tools     []MCPCatalogTool
}

// IsMCPToolEntry reports whether a pinned snapshot entry is an MCP-origin
// model tool (the population progressive disclosure defers).
func IsMCPToolEntry(entry RunToolAvailabilityEntry) bool {
	return entry.Origin == SourceMCP
}

// CollectMCPCatalog groups every MCP-origin snapshot entry by server. Order is
// deterministic: servers and tools sort by name.
func CollectMCPCatalog(snapshot *RunToolAvailabilitySnapshot) []MCPCatalogServer {
	if snapshot == nil {
		return nil
	}
	groups := map[string]*MCPCatalogServer{}
	for _, entry := range snapshot.Entries() {
		if !IsMCPToolEntry(entry) {
			continue
		}
		server := strings.TrimSpace(entry.Definition.Annotations.ServerName)
		if server == "" {
			server = mcpServerFromToolName(entry.Definition.Name)
		}
		group := groups[server]
		if group == nil {
			group = &MCPCatalogServer{Server: server, Readiness: entry.Readiness.Readiness}
			groups[server] = group
		}
		if entry.Readiness.Readiness != ProviderReady {
			group.Readiness = entry.Readiness.Readiness
		}
		rawName := strings.TrimSpace(entry.Definition.Annotations.ToolName)
		if rawName == "" {
			rawName = entry.Definition.Name
		}
		group.Tools = append(group.Tools, MCPCatalogTool{
			Name:        entry.Definition.Name,
			RawName:     rawName,
			Server:      server,
			Description: strings.TrimSpace(entry.Definition.Description),
			Readiness:   entry.Readiness.Readiness,
			InputSchema: entry.Definition.InputSchema,
		})
	}
	servers := make([]MCPCatalogServer, 0, len(groups))
	for _, group := range groups {
		sort.Slice(group.Tools, func(i, j int) bool { return group.Tools[i].Name < group.Tools[j].Name })
		servers = append(servers, *group)
	}
	sort.Slice(servers, func(i, j int) bool { return servers[i].Server < servers[j].Server })
	return servers
}

// mcpServerFromToolName derives the server identity from a registered MCP
// tool name when annotations are missing. Registered names always look like
// mcp__<server>__<tool>; the server segment itself may contain "__"
// (plugin__<plugin>__<server>), so only the outermost markers are trusted.
func mcpServerFromToolName(name string) string {
	trimmed := strings.TrimPrefix(name, "mcp__")
	if trimmed == name {
		return name
	}
	if index := strings.LastIndex(trimmed, "__"); index > 0 {
		return trimmed[:index]
	}
	return trimmed
}

// RenderMCPCatalog renders the deferred-tool directory with a rune budget.
// It degrades deterministically: per-tool descriptions shrink first, then
// drop to name-only listings, then to per-server counts, and finally trailing
// servers are omitted with an explicit truncation marker.
func RenderMCPCatalog(servers []MCPCatalogServer, budgetRunes int) string {
	if len(servers) == 0 {
		return ""
	}
	if budgetRunes <= 0 {
		budgetRunes = DefaultMCPCatalogBudgetRunes
	}
	for _, descriptionLimit := range []int{120, 64, 0} {
		rendered := renderMCPCatalogLevel(servers, descriptionLimit, false)
		if len([]rune(rendered)) <= budgetRunes {
			return rendered
		}
	}
	rendered := renderMCPCatalogLevel(servers, 0, true)
	if len([]rune(rendered)) <= budgetRunes {
		return rendered
	}
	// Even count-only lines exceed the budget: keep leading servers and mark
	// the truncation instead of silently dropping the rest.
	lines := strings.Split(renderMCPCatalogLevel(servers, 0, true), "\n")
	var kept []string
	used := 0
	const truncationMarker = "- …(catalog truncated; more MCP servers exist — search by keyword)"
	markerCost := len([]rune(truncationMarker)) + 1
	for _, line := range lines {
		cost := len([]rune(line)) + 1
		if used+cost+markerCost > budgetRunes {
			break
		}
		kept = append(kept, line)
		used += cost
	}
	kept = append(kept, truncationMarker)
	return strings.Join(kept, "\n")
}

func renderMCPCatalogLevel(servers []MCPCatalogServer, descriptionLimit int, countsOnly bool) string {
	var builder strings.Builder
	for serverIndex, server := range servers {
		if serverIndex > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("- ")
		builder.WriteString(server.Server)
		builder.WriteString(" (")
		builder.WriteString(itoa(len(server.Tools)))
		if len(server.Tools) == 1 {
			builder.WriteString(" tool")
		} else {
			builder.WriteString(" tools")
		}
		if server.Readiness != "" && server.Readiness != ProviderReady {
			builder.WriteString(", ")
			builder.WriteString(string(server.Readiness))
		}
		builder.WriteString(")")
		if countsOnly {
			continue
		}
		if descriptionLimit <= 0 {
			names := make([]string, 0, len(server.Tools))
			for _, item := range server.Tools {
				names = append(names, item.RawName)
			}
			builder.WriteString(": ")
			builder.WriteString(strings.Join(names, ", "))
			continue
		}
		for _, item := range server.Tools {
			builder.WriteString("\n  - ")
			builder.WriteString(item.RawName)
			description := compactWhitespace(item.Description)
			if description != "" {
				builder.WriteString(": ")
				builder.WriteString(truncateRunes(description, descriptionLimit))
			}
		}
	}
	return builder.String()
}

func compactWhitespace(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if limit <= 0 || len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// MCPSearchMatch is one scored search hit over the deferred MCP catalog.
type MCPSearchMatch struct {
	Tool  MCPCatalogTool
	Score int
}

// SearchMCPTools implements the deterministic keyword search used by
// mcp_find. Supported query forms:
//   - "select:<name1>,<name2>" resolves exact tools by registered or raw name;
//   - a bare registered/raw tool name resolves that tool directly;
//   - keywords score against name parts, server identity, descriptions and
//     parameter names; "+term" marks a required term.
func SearchMCPTools(servers []MCPCatalogServer, query string, limit int) []MCPSearchMatch {
	if limit <= 0 {
		limit = 5
	}
	flattened := make([]MCPCatalogTool, 0)
	for _, server := range servers {
		flattened = append(flattened, server.Tools...)
	}
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || len(flattened) == 0 {
		return nil
	}
	lower := strings.ToLower(trimmed)

	if selected, ok := strings.CutPrefix(lower, "select:"); ok {
		var matches []MCPSearchMatch
		seen := map[string]bool{}
		for _, requested := range strings.Split(selected, ",") {
			requested = strings.TrimSpace(requested)
			if requested == "" {
				continue
			}
			for _, item := range flattened {
				if !strings.EqualFold(item.Name, requested) && !strings.EqualFold(item.RawName, requested) {
					continue
				}
				if seen[item.Name] {
					break
				}
				seen[item.Name] = true
				matches = append(matches, MCPSearchMatch{Tool: item, Score: 100})
				break
			}
		}
		return matches
	}

	// Exact tool-name fast path mirrors direct selection without the prefix.
	for _, item := range flattened {
		if strings.EqualFold(item.Name, trimmed) || strings.EqualFold(item.RawName, trimmed) {
			return []MCPSearchMatch{{Tool: item, Score: 100}}
		}
	}

	terms := strings.Fields(lower)
	required := make([]string, 0)
	optional := make([]string, 0)
	for _, term := range terms {
		if stripped, ok := strings.CutPrefix(term, "+"); ok && stripped != "" {
			required = append(required, stripped)
			continue
		}
		optional = append(optional, term)
	}
	scoring := append(append([]string(nil), required...), optional...)
	if len(scoring) == 0 {
		return nil
	}

	matches := make([]MCPSearchMatch, 0)
	for _, item := range flattened {
		parts := mcpToolNameParts(item)
		description := strings.ToLower(item.Description)
		parameters := strings.ToLower(strings.Join(mcpParameterNames(item.InputSchema), " "))
		if len(required) > 0 {
			satisfied := true
			for _, term := range required {
				if !mcpTermMatches(term, parts, description, parameters) {
					satisfied = false
					break
				}
			}
			if !satisfied {
				continue
			}
		}
		score := 0
		for _, term := range scoring {
			switch {
			case containsString(parts, term):
				score += 12
			case anyContains(parts, term):
				score += 6
			}
			if strings.Contains(parameters, term) {
				score += 3
			}
			if containsWord(description, term) {
				score += 2
			}
		}
		if score > 0 {
			matches = append(matches, MCPSearchMatch{Tool: item, Score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Tool.Name < matches[j].Tool.Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func mcpToolNameParts(item MCPCatalogTool) []string {
	seen := map[string]bool{}
	parts := make([]string, 0)
	appendPart := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		parts = append(parts, value)
	}
	for _, source := range []string{item.Name, item.RawName, item.Server} {
		for _, segment := range splitIdentifier(source) {
			appendPart(segment)
		}
	}
	return parts
}

func splitIdentifier(value string) []string {
	// Break camelCase boundaries, then split on non-alphanumeric separators.
	var spaced strings.Builder
	runes := []rune(value)
	for index, r := range runes {
		if index > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[index-1]) {
			spaced.WriteRune(' ')
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			spaced.WriteRune(unicode.ToLower(r))
		} else {
			spaced.WriteRune(' ')
		}
	}
	return strings.Fields(spaced.String())
}

func mcpParameterNames(schema ToolParametersSchema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mcpTermMatches(term string, parts []string, description, parameters string) bool {
	return containsString(parts, term) || anyContains(parts, term) ||
		strings.Contains(parameters, term) || containsWord(description, term)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func anyContains(values []string, target string) bool {
	for _, value := range values {
		if strings.Contains(value, target) {
			return true
		}
	}
	return false
}

// containsWord reports whether target appears in text on word boundaries.
func containsWord(text, target string) bool {
	if text == "" || target == "" {
		return false
	}
	index := 0
	for {
		found := strings.Index(text[index:], target)
		if found < 0 {
			return false
		}
		start := index + found
		end := start + len(target)
		beforeOK := start == 0 || !isWordByte(text[start-1])
		afterOK := end >= len(text) || !isWordByte(text[end])
		if beforeOK && afterOK {
			return true
		}
		index = start + 1
		if index >= len(text) {
			return false
		}
	}
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// MCPActivationObserver receives the deferred MCP tool names a search call
// loaded. The runner persists the durable activation fact before returning;
// a persistence error fails the observing tool call.
type MCPActivationObserver func(ctx context.Context, names []string) error

type mcpActivationObserverContextKey struct{}

// ContextWithMCPActivationObserver binds the run-scoped activation observer
// consumed by the mcp_find tool.
func ContextWithMCPActivationObserver(ctx context.Context, observer MCPActivationObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, mcpActivationObserverContextKey{}, observer)
}

// MCPActivationObserverFromContext returns the bound observer, if any.
func MCPActivationObserverFromContext(ctx context.Context) MCPActivationObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(mcpActivationObserverContextKey{}).(MCPActivationObserver)
	return observer
}
