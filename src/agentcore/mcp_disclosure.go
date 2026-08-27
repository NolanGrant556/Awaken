package agentcore

import (
	"encoding/json"
	"strings"

	convcontext "awaken/src/context"
	"awaken/src/tool"
)

// mcpDisclosureProjection is the per-request-builder progressive-disclosure
// decision over the pinned run snapshot. Inputs (snapshot, model window,
// policy) are fixed per run, so the enablement decision cannot flip inside a
// run; only the deferred set shrinks as tools are activated.
type mcpDisclosureProjection struct {
	Enabled bool
	// Deferred holds MCP tool names whose definitions stay out of the model
	// tool list until activated.
	Deferred map[string]bool
	// Catalog is the rendered server/tool directory embedded in the mcp_find
	// description. It intentionally ignores the activated set so activations
	// remain append-only tool additions instead of catalog rewrites.
	Catalog string
	// Description is the full mcp_find model description (guidance + catalog).
	Description string
	// EstimatedMCPTokens is the local estimate for every MCP definition the
	// snapshot would otherwise send.
	EstimatedMCPTokens int
	// ThresholdTokens is the auto-mode activation threshold that was applied
	// (0 when the policy is always/off).
	ThresholdTokens int
}

// mcpFindGuidancePrefix carries the mandatory usage workflow next to the
// catalog, mirroring how the Skill catalog embeds its trigger rules.
const mcpFindGuidancePrefix = `Search deferred MCP tools and load their full definitions on demand.

The MCP servers below are connected, but their tool definitions are NOT loaded into context yet. To use one:
1. Call this tool with keywords (for example "github create issue"), "select:<full tool name>[,<more>]", or one exact tool name.
2. The result confirms the matched names and loads their full definitions into your tool list for the following turns.
3. On the next model turn, call the loaded tool directly by its full name using the schema in your tool list.
Calling a listed MCP tool directly without loading also works — its definition loads automatically for later turns — but prefer searching first so the exact input schema is known.

Connected MCP servers and deferred tools:
`

func buildMCPDisclosureProjection(
	snapshot *tool.RunToolAvailabilitySnapshot,
	modelConfig ModelConfig,
	activated []string,
	policy tool.MCPDisclosurePolicy,
) mcpDisclosureProjection {
	projection := mcpDisclosureProjection{}
	if snapshot == nil || policy.Mode == tool.MCPDisclosureOff {
		return projection
	}
	servers := tool.CollectMCPCatalog(snapshot)
	if len(servers) == 0 {
		return projection
	}
	projection.EstimatedMCPTokens = estimateMCPDefinitionTokens(snapshot, servers)
	if policy.Mode == tool.MCPDisclosureAuto {
		percent := policy.ThresholdPercent
		if percent <= 0 {
			percent = tool.DefaultMCPDeferralThresholdPercent
		}
		projection.ThresholdTokens = mcpDisclosureEffectiveWindow(modelConfig) * percent / 100
		if projection.EstimatedMCPTokens < projection.ThresholdTokens || projection.ThresholdTokens <= 0 {
			return projection
		}
	}

	activatedSet := make(map[string]bool, len(activated))
	for _, name := range activated {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			activatedSet[trimmed] = true
		}
	}
	deferred := map[string]bool{}
	for _, server := range servers {
		for _, item := range server.Tools {
			if !activatedSet[item.Name] {
				deferred[item.Name] = true
			}
		}
	}
	projection.Enabled = true
	projection.Deferred = deferred
	projection.Catalog = tool.RenderMCPCatalog(servers, tool.DefaultMCPCatalogBudgetRunes)
	projection.Description = mcpFindGuidancePrefix + projection.Catalog
	return projection
}

// estimateMCPDefinitionTokens estimates every MCP definition with the same
// schema shape getTools would send, so the auto threshold compares what the
// model would actually pay. The estimate covers all MCP tools (not only the
// currently deferred ones) to keep the decision stable across activations.
func estimateMCPDefinitionTokens(snapshot *tool.RunToolAvailabilitySnapshot, servers []tool.MCPCatalogServer) int {
	total := 0
	for _, server := range servers {
		for _, item := range server.Tools {
			entry, err := snapshot.Lookup(item.Name)
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
	}
	return total
}

// mcpDisclosureEffectiveWindow mirrors the context engine's window defaults
// without evaluating a full envelope.
func mcpDisclosureEffectiveWindow(config ModelConfig) int {
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
