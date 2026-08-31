package tool

import (
	"context"
	"os"
	"sort"
	"strings"
	"unicode"
)

// ToolFindToolName is the builtin auxiliary tool that searches deferred tool
// definitions (rarely used builtin auxiliary tools and MCP tools) and loads
// them on demand.
const ToolFindToolName = "tool_find"

// MCPDisclosureEnvVar overrides the MCP progressive-disclosure policy:
// "off"/"false"/"0" disables deferral, "always"/"true"/"1" always defers,
// "auto" or "auto:<percent>" defers only when the estimated MCP definition
// size exceeds <percent> of the model context window. Empty defaults to always
// deferring MCP definitions; auto remains an explicit compatibility mode.
const MCPDisclosureEnvVar = "AWAKEN_MCP_DEFERRED_TOOLS"

// BuiltinDisclosureEnvVar overrides the builtin auxiliary progressive-
// disclosure policy with the same grammar as MCPDisclosureEnvVar. It only
// governs the host-owned deferrable builtin auxiliary group; model base tools
// and core-loop auxiliaries are never deferred.
const BuiltinDisclosureEnvVar = "AWAKEN_BUILTIN_DEFERRED_TOOLS"

const (
	// DefaultDeferralThresholdPercent is the auto-mode activation threshold:
	// a group's deferred definitions must exceed this share of the context
	// window before deferral turns on. Small catalogs stay fully loaded.
	DefaultDeferralThresholdPercent = 10
	// DefaultMCPCatalogBudgetRunes bounds the rendered deferred-tool catalog
	// sections that are embedded in the tool_find tool description.
	DefaultMCPCatalogBudgetRunes = 8000
	// DefaultBuiltinCatalogBudgetRunes bounds the deferred builtin auxiliary
	// catalog section. The group is small, so the budget stays tight.
	DefaultBuiltinCatalogBudgetRunes = 2400
	// builtinCatalogSummaryRunes bounds one builtin catalog line summary.
	builtinCatalogSummaryRunes = 160
)

// DisclosureMode is the resolved deferral policy mode.
type DisclosureMode string

const (
	DisclosureAuto   DisclosureMode = "auto"
	DisclosureAlways DisclosureMode = "always"
	DisclosureOff    DisclosureMode = "off"
)

// DisclosurePolicy is the per-process deferral policy for one tool group. It
// is resolved once per request build from the environment; the run snapshot
// and context window keep the resulting decision stable inside one run.
type DisclosurePolicy struct {
	Mode             DisclosureMode
	ThresholdPercent int
}

// ResolveDisclosurePolicy parses a raw policy value. Unknown values fall
// back to the default always-on policy so a typo cannot silently restore the
// full deferred schema set to every model request.
func ResolveDisclosurePolicy(raw string) DisclosurePolicy {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "":
		return DisclosurePolicy{Mode: DisclosureAlways}
	case "auto":
		return DisclosurePolicy{Mode: DisclosureAuto, ThresholdPercent: DefaultDeferralThresholdPercent}
	case "off", "false", "0", "disabled":
		return DisclosurePolicy{Mode: DisclosureOff}
	case "always", "true", "1", "on":
		return DisclosurePolicy{Mode: DisclosureAlways}
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
				return DisclosurePolicy{Mode: DisclosureAlways}
			case 100:
				return DisclosurePolicy{Mode: DisclosureOff}
			default:
				return DisclosurePolicy{Mode: DisclosureAuto, ThresholdPercent: parsed}
			}
		}
	}
	return DisclosurePolicy{Mode: DisclosureAlways}
}

// MCPDisclosurePolicyFromEnv resolves the MCP group policy from the
// environment.
func MCPDisclosurePolicyFromEnv() DisclosurePolicy {
	return ResolveDisclosurePolicy(os.Getenv(MCPDisclosureEnvVar))
}

// BuiltinDisclosurePolicyFromEnv resolves the builtin auxiliary group policy
// from the environment.
func BuiltinDisclosurePolicyFromEnv() DisclosurePolicy {
	return ResolveDisclosurePolicy(os.Getenv(BuiltinDisclosureEnvVar))
}

// deferrableBuiltinToolNames is the host-owned builtin auxiliary deferral
// group: rarely used, schema-heavy tools whose invocation normally follows an
// explicit user intent or a tool result that names them. Core-loop tools
// (model base tools, subagent, update_plan, ask_user, artifact_publish,
// memory, trash_file, skill runtime tools, computer/media tools) are never
// listed here.
var deferrableBuiltinToolNames = []string{
	"archived_history",
	"thread_history_read",
	"calculator",
	"configure_acp_runner",
	"configure_hook",
	"delegate_external_agent",
	"dependency_install",
	"git_remote",
	"plugin",
	"skill_install",
	"skill_package_export",
}

// DeferrableBuiltinToolNames returns the ordered host-owned builtin auxiliary
// deferral group.
func DeferrableBuiltinToolNames() []string {
	return append([]string(nil), deferrableBuiltinToolNames...)
}

// IsDeferrableBuiltinToolName reports whether name belongs to the host-owned
// builtin auxiliary deferral group.
func IsDeferrableBuiltinToolName(name string) bool {
	for _, candidate := range deferrableBuiltinToolNames {
		if candidate == name {
			return true
		}
	}
	return false
}

// BuiltinCatalogTool is one deferred-capable builtin auxiliary tool observed
// in the pinned run snapshot after exposure filtering.
type BuiltinCatalogTool struct {
	Name        string
	Summary     string
	Description string
	Readiness   ProviderReadiness
	InputSchema ToolParametersSchema
}

// IsDeferrableBuiltinEntry reports whether a pinned snapshot entry belongs to
// the builtin auxiliary deferral group.
func IsDeferrableBuiltinEntry(entry RunToolAvailabilityEntry) bool {
	return entry.Origin == SourceBuiltin && IsDeferrableBuiltinToolName(entry.Definition.Name)
}

// CollectBuiltinDeferredCatalog lists the deferrable builtin auxiliary tools
// present in the pinned snapshot that remain visible under the run's exposure
// policy. Hidden tools never enter the catalog, the search space, or the
// model-facing directory. Order is deterministic by tool name.
func CollectBuiltinDeferredCatalog(snapshot *RunToolAvailabilitySnapshot, exposure ToolExposurePolicy) []BuiltinCatalogTool {
	if snapshot == nil {
		return nil
	}
	entries := make([]BuiltinCatalogTool, 0)
	for _, entry := range snapshot.Entries() {
		if !IsDeferrableBuiltinEntry(entry) {
			continue
		}
		name := entry.Definition.Name
		if !IsToolExposed(name, exposure) {
			continue
		}
		description := strings.TrimSpace(entry.Definition.Description)
		entries = append(entries, BuiltinCatalogTool{
			Name:        name,
			Summary:     builtinCatalogSummary(entry.Definition),
			Description: description,
			Readiness:   entry.Readiness.Readiness,
			InputSchema: entry.Definition.InputSchema,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// builtinCatalogSummary compiles the one-line model-facing summary for a
// deferred builtin tool: the first sentence of the description (falling back
// to the purpose), deterministically truncated.
func builtinCatalogSummary(definition ToolDefinition) string {
	source := strings.TrimSpace(definition.Description)
	if source == "" {
		source = strings.TrimSpace(definition.Purpose)
	}
	source = strings.Join(strings.Fields(source), " ")
	if index := strings.Index(source, ". "); index > 0 {
		source = source[:index+1]
	}
	runes := []rune(source)
	if len(runes) > builtinCatalogSummaryRunes {
		source = string(runes[:builtinCatalogSummaryRunes-1]) + "…"
	}
	return source
}

// RenderBuiltinCatalog renders the deferred builtin auxiliary directory with
// a rune budget. It degrades deterministically from name+summary lines to
// name-only lines, then keeps leading entries with an explicit truncation
// marker.
func RenderBuiltinCatalog(entries []BuiltinCatalogTool, budgetRunes int) string {
	if len(entries) == 0 {
		return ""
	}
	if budgetRunes <= 0 {
		budgetRunes = DefaultBuiltinCatalogBudgetRunes
	}
	rendered := renderBuiltinCatalogLevel(entries, false)
	if len([]rune(rendered)) <= budgetRunes {
		return rendered
	}
	rendered = renderBuiltinCatalogLevel(entries, true)
	if len([]rune(rendered)) <= budgetRunes {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	var kept []string
	used := 0
	const truncationMarker = "- …(catalog truncated; more deferred builtin tools exist — search by keyword)"
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

func renderBuiltinCatalogLevel(entries []BuiltinCatalogTool, namesOnly bool) string {
	var builder strings.Builder
	for index, entry := range entries {
		if index > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString("- ")
		builder.WriteString(entry.Name)
		if entry.Readiness != "" && entry.Readiness != ProviderReady {
			builder.WriteString(" (")
			builder.WriteString(string(entry.Readiness))
			builder.WriteString(")")
		}
		if namesOnly || entry.Summary == "" {
			continue
		}
		builder.WriteString(": ")
		builder.WriteString(entry.Summary)
	}
	return builder.String()
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
// model tool (the population MCP progressive disclosure defers).
func IsMCPToolEntry(entry RunToolAvailabilityEntry) bool {
	return entry.Origin == SourceMCP
}

// CollectMCPCatalog groups every MCP-origin snapshot entry that remains
// visible under the run's exposure policy by server. Order is deterministic:
// servers and tools sort by name.
func CollectMCPCatalog(snapshot *RunToolAvailabilitySnapshot, exposure ToolExposurePolicy) []MCPCatalogServer {
	if snapshot == nil {
		return nil
	}
	groups := map[string]*MCPCatalogServer{}
	for _, entry := range snapshot.Entries() {
		if !IsMCPToolEntry(entry) {
			continue
		}
		if !IsToolExposed(entry.Definition.Name, exposure) {
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

// RenderMCPCatalog renders a name-only deferred-tool directory with a rune
// budget. Full descriptions and schemas stay in the local search index. The
// model-facing directory degrades deterministically from names to per-server
// counts, then omits trailing servers with an explicit truncation marker.
func RenderMCPCatalog(servers []MCPCatalogServer, budgetRunes int) string {
	if len(servers) == 0 {
		return ""
	}
	if budgetRunes <= 0 {
		budgetRunes = DefaultMCPCatalogBudgetRunes
	}
	rendered := renderMCPCatalogLevel(servers, false)
	if len([]rune(rendered)) <= budgetRunes {
		return rendered
	}
	rendered = renderMCPCatalogLevel(servers, true)
	if len([]rune(rendered)) <= budgetRunes {
		return rendered
	}
	// Even count-only lines exceed the budget: keep leading servers and mark
	// the truncation instead of silently dropping the rest.
	lines := strings.Split(renderMCPCatalogLevel(servers, true), "\n")
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

func renderMCPCatalogLevel(servers []MCPCatalogServer, countsOnly bool) string {
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
		names := make([]string, 0, len(server.Tools))
		for _, item := range server.Tools {
			names = append(names, item.RawName)
		}
		builder.WriteString(": ")
		builder.WriteString(strings.Join(names, ", "))
	}
	return builder.String()
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

// DeferredToolSearchSpace is the run-scoped, exposure-filtered population the
// tool_find tool searches. The request builder computes it from the same
// pinned snapshot and enablement decision that shaped the model projection,
// so the search space always equals what the model was told is deferred.
type DeferredToolSearchSpace struct {
	Builtin []BuiltinCatalogTool
	Servers []MCPCatalogServer
}

// TotalDeferredTools counts every searchable deferred tool in the space.
func (s DeferredToolSearchSpace) TotalDeferredTools() int {
	total := len(s.Builtin)
	for _, server := range s.Servers {
		total += len(server.Tools)
	}
	return total
}

// DeferredToolMatch is one scored search hit over the deferred tool space.
type DeferredToolMatch struct {
	Name      string
	Origin    ToolSource
	Server    string
	Readiness ProviderReadiness
	Score     int
}

type deferredSearchEntry struct {
	name        string
	rawName     string
	origin      ToolSource
	server      string
	readiness   ProviderReadiness
	description string
	schema      ToolParametersSchema
}

func flattenDeferredSearchSpace(space DeferredToolSearchSpace) []deferredSearchEntry {
	flattened := make([]deferredSearchEntry, 0, len(space.Builtin))
	for _, item := range space.Builtin {
		flattened = append(flattened, deferredSearchEntry{
			name:        item.Name,
			rawName:     item.Name,
			origin:      SourceBuiltin,
			readiness:   item.Readiness,
			description: item.Description,
			schema:      item.InputSchema,
		})
	}
	for _, server := range space.Servers {
		for _, item := range server.Tools {
			flattened = append(flattened, deferredSearchEntry{
				name:        item.Name,
				rawName:     item.RawName,
				origin:      SourceMCP,
				server:      item.Server,
				readiness:   item.Readiness,
				description: item.Description,
				schema:      item.InputSchema,
			})
		}
	}
	return flattened
}

func matchFromEntry(entry deferredSearchEntry, score int) DeferredToolMatch {
	return DeferredToolMatch{
		Name:      entry.name,
		Origin:    entry.origin,
		Server:    entry.server,
		Readiness: entry.readiness,
		Score:     score,
	}
}

// SearchDeferredTools implements the deterministic keyword search used by
// tool_find over both deferred groups. Supported query forms:
//   - "select:<name1>,<name2>" resolves exact tools by registered or raw name;
//   - a bare registered/raw tool name resolves that tool directly;
//   - keywords score against name parts, server identity, descriptions and
//     parameter names; "+term" marks a required term.
func SearchDeferredTools(space DeferredToolSearchSpace, query string, limit int) []DeferredToolMatch {
	if limit <= 0 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	flattened := flattenDeferredSearchSpace(space)
	trimmed := strings.TrimSpace(query)
	if trimmed == "" || len(flattened) == 0 {
		return nil
	}
	lower := strings.ToLower(trimmed)

	if selected, ok := strings.CutPrefix(lower, "select:"); ok {
		var matches []DeferredToolMatch
		seen := map[string]bool{}
		for _, requested := range strings.Split(selected, ",") {
			if len(matches) >= limit {
				break
			}
			requested = strings.TrimSpace(requested)
			if requested == "" {
				continue
			}
			for _, item := range flattened {
				if !strings.EqualFold(item.name, requested) && !strings.EqualFold(item.rawName, requested) {
					continue
				}
				if seen[item.name] {
					break
				}
				seen[item.name] = true
				matches = append(matches, matchFromEntry(item, 100))
				break
			}
		}
		return matches
	}

	// Exact tool-name fast path mirrors direct selection without the prefix.
	for _, item := range flattened {
		if strings.EqualFold(item.name, trimmed) || strings.EqualFold(item.rawName, trimmed) {
			return []DeferredToolMatch{matchFromEntry(item, 100)}
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

	matches := make([]DeferredToolMatch, 0)
	for _, item := range flattened {
		parts := deferredToolNameParts(item)
		description := strings.ToLower(item.description)
		parameters := strings.ToLower(strings.Join(schemaParameterNames(item.schema), " "))
		if len(required) > 0 {
			satisfied := true
			for _, term := range required {
				if !deferredTermMatches(term, parts, description, parameters) {
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
			matches = append(matches, matchFromEntry(item, score))
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Name < matches[j].Name
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func deferredToolNameParts(item deferredSearchEntry) []string {
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
	for _, source := range []string{item.name, item.rawName, item.server} {
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

func schemaParameterNames(schema ToolParametersSchema) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func deferredTermMatches(term string, parts []string, description, parameters string) bool {
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

// DeferredToolActivationObserver receives the deferred tool names a search
// call loaded. The runner persists the durable activation fact before
// returning; a persistence error fails the observing tool call.
type DeferredToolActivationObserver func(ctx context.Context, names []string) error

type deferredToolActivationObserverContextKey struct{}

// ContextWithDeferredToolActivationObserver binds the run-scoped activation
// observer consumed by the tool_find tool.
func ContextWithDeferredToolActivationObserver(ctx context.Context, observer DeferredToolActivationObserver) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, deferredToolActivationObserverContextKey{}, observer)
}

// DeferredToolActivationObserverFromContext returns the bound observer, if
// any.
func DeferredToolActivationObserverFromContext(ctx context.Context) DeferredToolActivationObserver {
	if ctx == nil {
		return nil
	}
	observer, _ := ctx.Value(deferredToolActivationObserverContextKey{}).(DeferredToolActivationObserver)
	return observer
}

type deferredToolSearchSpaceContextKey struct{}

// ContextWithDeferredToolSearchSpace binds the run-scoped, exposure-filtered
// deferred tool search space consumed by the tool_find tool.
func ContextWithDeferredToolSearchSpace(ctx context.Context, space DeferredToolSearchSpace) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, deferredToolSearchSpaceContextKey{}, space)
}

// DeferredToolSearchSpaceFromContext returns the bound search space, if any.
func DeferredToolSearchSpaceFromContext(ctx context.Context) (DeferredToolSearchSpace, bool) {
	if ctx == nil {
		return DeferredToolSearchSpace{}, false
	}
	space, ok := ctx.Value(deferredToolSearchSpaceContextKey{}).(DeferredToolSearchSpace)
	return space, ok
}
