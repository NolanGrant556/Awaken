package agentcore

import (
	"encoding/json"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// toolDisclosureProjection is the per-request-builder progressive-disclosure
// decision over the pinned run snapshot for both deferrable groups (builtin
// auxiliary tools and MCP tools). Inputs (snapshot, model window, policies,
// exposure) are fixed per run, so the enablement decision cannot flip inside
// a run; only the deferred sets shrink as tools are activated.
type toolDisclosureProjection struct {
	MCPEnabled     bool
	BuiltinEnabled bool
	// DeferredMCP holds MCP tool names whose definitions stay out of the
	// model tool list until activated.
	DeferredMCP map[string]bool
	// DeferredBuiltin holds deferrable builtin auxiliary tool names whose
	// definitions stay out of the model tool list until activated.
	DeferredBuiltin map[string]bool
	// MCPCatalog is the rendered server/tool directory embedded in the
	// tool_find description. It intentionally ignores the activated set so
	// activations remain append-only tool additions instead of catalog
	// rewrites.
	MCPCatalog string
	// BuiltinCatalog is the rendered deferred builtin directory embedded in
	// the tool_find description, with the same byte-stability rule.
	BuiltinCatalog string
	// Description is the full tool_find model description (guidance +
	// catalogs).
	Description string
	// SearchSpace is the exposure-filtered population tool_find searches. It
	// matches the embedded catalogs exactly, including still-connecting MCP
	// servers, and ignores activation state.
	SearchSpace tool.DeferredToolSearchSpace
	// EstimatedMCPTokens is the local estimate for every MCP definition the
	// snapshot would otherwise send.
	EstimatedMCPTokens int
	// MCPThresholdTokens is the auto-mode activation threshold that was
	// applied to the MCP group (0 when the policy is always/off).
	MCPThresholdTokens int
	// EstimatedBuiltinTokens is the local estimate for every deferrable
	// builtin definition the snapshot would otherwise send.
	EstimatedBuiltinTokens int
	// BuiltinThresholdTokens is the auto-mode activation threshold that was
	// applied to the builtin group (0 when the policy is always/off).
	BuiltinThresholdTokens int
}

// Enabled reports whether any deferral group is active, which controls
// whether tool_find enters the model projection.
func (p toolDisclosureProjection) Enabled() bool {
	return p.MCPEnabled || p.BuiltinEnabled
}

// Deferred reports whether the named tool's definition is currently withheld
// from the model tool list.
func (p toolDisclosureProjection) Deferred(name string) bool {
	return p.DeferredMCP[name] || p.DeferredBuiltin[name]
}

// toolFindGuidancePrefix carries the mandatory usage workflow next to the
// catalogs, mirroring how the Skill catalog embeds its trigger rules. It must
// state explicitly that catalog lines are hints, not answers: models
// otherwise answer state questions (current Hook config, archived chats,
// installed plugins) from the one-line summary instead of loading and calling
// the owning tool.
const toolFindGuidancePrefix = `Search deferred tools and load their full definitions on demand.

The tools below are real, callable capabilities of this run. Only their full definitions are NOT loaded into context yet; each catalog line is a discovery hint, never an answer. To use one:
1. Call this tool with keywords (for example "github create issue"), "select:<full tool name>[,<more>]", or one exact tool name.
2. The result confirms the matched names and loads their full definitions into your tool list for the following turns.
3. On the next model turn, call the loaded tool directly by its full name using the schema in your tool list.
Calling a listed tool directly without loading also works — its definition loads automatically for later turns — but prefer searching first so the exact input schema is known.
MANDATORY: when the user asks about or wants to change state that a deferred tool owns (for example current Hook configuration, archived chats, plugins, ACP runners), you MUST load and call that tool instead of answering from this catalog or from memory. Do not claim such state is empty, missing, or unsupported without calling the owning tool first.
`

const toolFindBuiltinSectionHeader = "\nDeferred builtin tools:\n"

const toolFindMCPSectionHeader = "\nConnected MCP servers and deferred tools:\n"

func buildToolDisclosureProjection(
	snapshot *tool.RunToolAvailabilitySnapshot,
	modelConfig ModelConfig,
	activated []string,
	exposure tool.ToolExposurePolicy,
	mcpPolicy tool.DisclosurePolicy,
	builtinPolicy tool.DisclosurePolicy,
) toolDisclosureProjection {
	projection := toolDisclosureProjection{}
	if snapshot == nil {
		return projection
	}
	activatedSet := make(map[string]bool, len(activated))
	for _, name := range activated {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			activatedSet[trimmed] = true
		}
	}

	if mcpPolicy.Mode != tool.DisclosureOff {
		servers := tool.CollectMCPCatalog(snapshot, exposure)
		if len(servers) > 0 {
			names := make([]string, 0)
			for _, server := range servers {
				for _, item := range server.Tools {
					names = append(names, item.Name)
				}
			}
			projection.EstimatedMCPTokens = estimateDefinitionTokens(snapshot, names)
			enabled := true
			if mcpPolicy.Mode == tool.DisclosureAuto {
				percent := mcpPolicy.ThresholdPercent
				if percent <= 0 {
					percent = tool.DefaultDeferralThresholdPercent
				}
				projection.MCPThresholdTokens = disclosureEffectiveWindow(modelConfig) * percent / 100
				if projection.EstimatedMCPTokens < projection.MCPThresholdTokens || projection.MCPThresholdTokens <= 0 {
					enabled = false
				}
			}
			if enabled {
				deferred := map[string]bool{}
				for _, name := range names {
					if !activatedSet[name] {
						deferred[name] = true
					}
				}
				projection.MCPEnabled = true
				projection.DeferredMCP = deferred
				projection.MCPCatalog = tool.RenderMCPCatalog(servers, tool.DefaultMCPCatalogBudgetRunes)
				projection.SearchSpace.Servers = servers
			}
		}
	}

	if builtinPolicy.Mode != tool.DisclosureOff {
		entries := tool.CollectBuiltinDeferredCatalog(snapshot, exposure)
		if len(entries) > 0 {
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name)
			}
			projection.EstimatedBuiltinTokens = estimateDefinitionTokens(snapshot, names)
			enabled := true
			if builtinPolicy.Mode == tool.DisclosureAuto {
				percent := builtinPolicy.ThresholdPercent
				if percent <= 0 {
					percent = tool.DefaultDeferralThresholdPercent
				}
				projection.BuiltinThresholdTokens = disclosureEffectiveWindow(modelConfig) * percent / 100
				if projection.EstimatedBuiltinTokens < projection.BuiltinThresholdTokens || projection.BuiltinThresholdTokens <= 0 {
					enabled = false
				}
			}
			if enabled {
				deferred := map[string]bool{}
				for _, name := range names {
					if !activatedSet[name] {
						deferred[name] = true
					}
				}
				projection.BuiltinEnabled = true
				projection.DeferredBuiltin = deferred
				projection.BuiltinCatalog = tool.RenderBuiltinCatalog(entries, tool.DefaultBuiltinCatalogBudgetRunes)
				projection.SearchSpace.Builtin = entries
			}
		}
	}

	if !projection.Enabled() {
		return projection
	}
	var description strings.Builder
	description.WriteString(toolFindGuidancePrefix)
	if projection.BuiltinEnabled && projection.BuiltinCatalog != "" {
		description.WriteString(toolFindBuiltinSectionHeader)
		description.WriteString(projection.BuiltinCatalog)
		description.WriteString("\n")
	}
	if projection.MCPEnabled && projection.MCPCatalog != "" {
		description.WriteString(toolFindMCPSectionHeader)
		description.WriteString(projection.MCPCatalog)
		description.WriteString("\n")
	}
	projection.Description = strings.TrimSuffix(description.String(), "\n")
	return projection
}

// estimateDefinitionTokens estimates the named definitions with the same
// schema shape getTools would send, so the auto threshold compares what the
// model would actually pay. The estimate covers the whole group (not only the
// currently deferred part) to keep the decision stable across activations.
func estimateDefinitionTokens(snapshot *tool.RunToolAvailabilitySnapshot, names []string) int {
	total := 0
	for _, name := range names {
		entry, err := snapshot.Lookup(name)
		if err != nil {
			continue
		}
		schema := tool.ToolSchema{
			Type: "function",
			Function: tool.ToolFunctionSchema{
				Name:        entry.Definition.Name,
				Description: tool.ModelDescription(entry.Definition),
				Parameters:  entry.Definition.InputSchema,
			},
		}
		if data, err := json.Marshal(schema); err == nil {
			total += convcontext.EstimateTokens(string(data))
		}
	}
	return total
}

// disclosureEffectiveWindow mirrors the context engine's window defaults
// without evaluating a full envelope.
func disclosureEffectiveWindow(config ModelConfig) int {
	windowConfig := contextWindowConfigFromModelConfig(config)
	modelWindow := defaultModelWindow
	if windowConfig.ModelWindow > 0 {
		modelWindow = windowConfig.ModelWindow
	}
	outputReserve := defaultOutputReserve
	if windowConfig.OutputReserve > 0 {
		outputReserve = windowConfig.OutputReserve
	}
	if effective := modelWindow - outputReserve; effective > 0 {
		return effective
	}
	return 0
}
