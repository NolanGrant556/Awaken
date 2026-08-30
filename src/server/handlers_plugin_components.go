package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"awaken/src/hookruntime"
	"awaken/src/pluginagents"
	"awaken/src/plugincommands"
	"awaken/src/pluginhooks"
	"awaken/src/plugins"
	"awaken/src/plugins/ir"
	awruntime "awaken/src/runtime"
	"awaken/src/subagentruntime"
)

func componentSummary(pluginName string, component ir.Component) map[string]any {
	diagnostics := component.Diagnostics
	if diagnostics == nil {
		diagnostics = []ir.Diagnostic{}
	}
	summary := map[string]any{
		"logicalId":          component.LogicalID,
		"logicalIdEncoded":   component.LogicalIDEncoded,
		"revisionId":         component.RevisionID,
		"kind":               component.Kind,
		"displayName":        component.DisplayName,
		"description":        component.Description,
		"sourceDialect":      component.SourceDialect,
		"sourceManifestPath": component.SourceManifestPath,
		"sourcePath":         component.SourcePath,
		"mappingState":       component.Mapping,
		"validation":         component.Validation,
		"diagnostics":        diagnostics,
	}
	if component.Skill != nil {
		skillName := strings.TrimSpace(component.DeclaredName)
		if skillName == "" {
			skillName = strings.TrimSpace(component.DerivedName)
		}
		summary["skill"] = map[string]any{
			"path":      component.Skill.Path,
			"catalogId": plugins.SkillCatalogID(pluginName, skillName),
		}
	}
	if component.MCP != nil {
		summary["mcp"] = map[string]any{"shortName": component.MCP.ShortName, "transport": component.MCP.Transport}
	}
	if component.Rule != nil {
		summary["rule"] = map[string]any{"activation": component.Rule.Activation, "globs": component.Rule.Globs, "scope": component.Rule.Scope}
	}
	if component.Agent != nil {
		budget := pluginagents.EffectiveRuntimeBudget(component.Agent)
		agentSummary := map[string]any{
			"model": component.Agent.Model, "readOnly": component.Agent.ReadonlyHint,
		}
		if budget.MaxTurns > 0 {
			agentSummary["maxTurns"] = budget.MaxTurns
		}
		if budget.Timeout > 0 {
			agentSummary["timeoutSeconds"] = int(budget.Timeout.Seconds())
		}
		summary["agent"] = agentSummary
	}
	if component.Command != nil {
		acceptsArguments, argumentsRequired := plugincommands.ArgumentPolicy(component.Command)
		summary["command"] = map[string]any{"invocationName": component.Command.InvocationName, "argumentHint": component.Command.ArgumentHint, "acceptsArguments": acceptsArguments, "argumentsRequired": argumentsRequired}
	}
	if component.Hook != nil {
		async, asyncRewake := pluginhooks.EffectiveAsync(component.Hook)
		summary["hook"] = map[string]any{"sourceEvent": component.Hook.SourceEvent, "mappedEvent": component.Hook.MappedEvent, "matcher": component.Hook.Matcher, "condition": component.Hook.Condition, "handlerType": component.Hook.HandlerType, "scope": component.Hook.Scope, "timeoutSeconds": float64(pluginhooks.EffectiveTimeoutMS(component.Hook)) / 1000, "async": async, "asyncRewake": asyncRewake}
	}
	if component.Variable != nil {
		summary["variable"] = map[string]any{"key": component.Variable.Key, "type": component.Variable.Type, "required": component.Variable.Required, "secret": component.Variable.Secret}
	}
	if component.App != nil {
		summary["app"] = map[string]any{"connectorId": component.App.ConnectorID}
	}
	return summary
}

func componentStateSummary(pluginName string, state plugins.ComponentState) map[string]any {
	return map[string]any{
		"component":          componentSummary(pluginName, state.Component),
		"desiredState":       state.DesiredState,
		"ruleActivation":     state.RuleActivation,
		"configurationState": state.Configuration,
		"runtimeState":       state.Runtime,
		"integrityState":     state.Integrity,
		"mappingState":       state.Mapping,
		"switchable":         state.Switchable,
	}
}

func applyEffectiveComponentBudget(component ir.Component, properties map[string]any) {
	if component.Agent != nil {
		budget := pluginagents.EffectiveRuntimeBudget(component.Agent)
		if agentProperties, ok := properties["agent"].(map[string]any); ok {
			delete(agentProperties, "maxTurns")
			delete(agentProperties, "timeoutSeconds")
			if budget.MaxTurns > 0 {
				agentProperties["declaredMaxTurns"] = budget.MaxTurns
			}
			if budget.Timeout > 0 {
				agentProperties["declaredTimeoutSeconds"] = int(budget.Timeout.Seconds())
			}
		}
		delete(properties, "runtimeBudget")
	}
	if component.Hook != nil {
		if hookProperties, ok := properties["hook"].(map[string]any); ok {
			delete(hookProperties, "timeoutSeconds")
		}
		properties["runtimeBudget"] = map[string]any{
			"timeoutSeconds": float64(pluginhooks.EffectiveTimeoutMS(component.Hook)) / 1000,
		}
	}
}

func (h pluginHandlers) handleComponents(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	components, err := h.store.ComponentInventory(strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(components))
	pluginName := strings.TrimSpace(r.PathValue("name"))
	for _, component := range components {
		summary := componentStateSummary(pluginName, component)
		if component.Component.MCP != nil && h.manager != nil {
			identity := plugins.ServerIdentity(pluginName, component.Component.MCP.ShortName)
			componentSummaryValue, _ := summary["component"].(map[string]any)
			mcpSummary, _ := componentSummaryValue["mcp"].(map[string]any)
			if snapshot, snapshotErr := h.manager.SnapshotServer(identity); snapshotErr == nil {
				summary["runtimeState"] = snapshot.State
				mcpSummary["connectionState"] = snapshot.State
				mcpSummary["toolsLoaded"] = snapshot.ToolsLoaded
				mcpSummary["toolCount"] = snapshot.ToolCount
			}
		}
		out = append(out, summary)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h pluginHandlers) handleComponentDetail(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	detail, err := h.store.ComponentDetail(strings.TrimSpace(r.PathValue("name")), logicalID)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if kind := strings.TrimSpace(r.PathValue("kind")); kind != "" && string(detail.State.Component.Kind) != kind {
		writeErrorJSON(w, http.StatusNotFound, "COMPONENT_NOT_FOUND", "component kind does not match")
		return
	}
	if detail.Properties == nil {
		detail.Properties = map[string]any{}
	}
	applyEffectiveComponentBudget(detail.State.Component, detail.Properties)
	if detail.State.Component.MCP != nil && h.manager != nil {
		identity := plugins.ServerIdentity(strings.TrimSpace(r.PathValue("name")), detail.State.Component.MCP.ShortName)
		if snapshot, snapErr := h.manager.SnapshotServer(identity); snapErr == nil {
			detail.Properties["connection"] = snapshot
			if snapshot.ToolsLoaded {
				if tools, toolsErr := h.manager.ServerTools(identity); toolsErr == nil {
					detail.Properties["tools"] = tools
				}
			} else {
				detail.Properties["toolsState"] = "tools_not_loaded"
			}
		}
	}
	response := map[string]any{
		"state":          componentStateSummary(strings.TrimSpace(r.PathValue("name")), detail.State),
		"sourceEditable": detail.SourceEditable,
		"properties":     detail.Properties,
		"resources":      detail.Resources,
	}
	if detail.Preview != "" {
		response["preview"] = detail.Preview
	}
	if detail.Source != "" {
		response["source"] = detail.Source
	}
	writeJSON(w, http.StatusOK, response)
}

func (h pluginHandlers) handleComponentSource(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	if kind := strings.TrimSpace(r.PathValue("kind")); kind != string(ir.KindMCPServer) {
		writeErrorJSON(w, http.StatusConflict, "COMPONENT_SOURCE_NOT_EDITABLE", "only MCP source files are editable")
		return
	}
	var request struct {
		ExpectedDigest string `json:"expectedDigest"`
		Source         string `json:"source"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if strings.TrimSpace(request.ExpectedDigest) == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "expectedDigest is required")
		return
	}
	result, err := h.store.UpdateMCPSource(
		strings.TrimSpace(r.PathValue("name")),
		logicalID,
		request.ExpectedDigest,
		request.Source,
	)
	if err != nil {
		if errors.Is(err, plugins.ErrMCPInvalid) || errors.Is(err, plugins.ErrManifestInvalid) || errors.Is(err, plugins.ErrPathEscapes) {
			writeErrorJSON(w, http.StatusBadRequest, "MCP_SOURCE_INVALID", err.Error())
			return
		}
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plugin":   h.toPluginInfo(result.Plugin, true),
		"revision": h.store.Revision(),
		"changed":  result.Changed,
	})
}

func (h pluginHandlers) handleComponentState(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	plugin, err := h.store.SetComponentDesired(strings.TrimSpace(r.PathValue("name")), logicalID, request.Enabled)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, map[string]any{"plugin": h.toPluginInfo(plugin, true), "revision": h.store.Revision()})
}

func (h pluginHandlers) handleRuleActivation(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	var request struct {
		Activation string `json:"activation"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	plugin, err := h.store.SetRuleActivation(strings.TrimSpace(r.PathValue("name")), logicalID, request.Activation)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugin": h.toPluginInfo(plugin, true), "revision": h.store.Revision()})
}

type pluginCommandRequest struct {
	ExpectedDigest string                     `json:"expectedDigest"`
	Arguments      []string                   `json:"arguments"`
	Run            *awruntime.StartRunRequest `json:"run,omitempty"`
}

func (h pluginHandlers) expandPluginCommand(r *http.Request) (plugincommands.Expansion, pluginCommandRequest, error) {
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		return plugincommands.Expansion{}, pluginCommandRequest{}, err
	}
	var request pluginCommandRequest
	if err := decodeJSON(r, &request); err != nil {
		return plugincommands.Expansion{}, request, err
	}
	expansion, err := h.store.ExpandCommand(strings.TrimSpace(r.PathValue("name")), strings.TrimSpace(request.ExpectedDigest), logicalID, request.Arguments)
	return expansion, request, err
}

func (h pluginHandlers) handleCommandExpand(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	expansion, _, err := h.expandPluginCommand(r)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMMAND_EXPAND_FAILED", err.Error())
		return
	}
	expansion.Revision = h.store.Revision()
	writeJSON(w, http.StatusOK, expansion)
}

func (h pluginHandlers) handleCommandInvoke(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUNTIME_UNAVAILABLE", "run service is unavailable")
		return
	}
	expansion, request, err := h.expandPluginCommand(r)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMMAND_EXPAND_FAILED", err.Error())
		return
	}
	if request.Run == nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "run is required")
		return
	}
	runRequest := *request.Run
	runRequest.Input = expansion.Prompt
	name := strings.TrimSpace(r.PathValue("name"))
	runRequest.ComposerTokens = append(runRequest.ComposerTokens, awruntime.ComposerToken{
		ID: "plugin-command:" + name + ":" + strings.TrimSpace(request.ExpectedDigest), Type: "plugin", Label: name,
		Payload: awruntime.ComposerTokenPayload{PluginName: name, PluginDigest: strings.TrimSpace(request.ExpectedDigest)},
	})
	run, err := h.runService.StartRun(runRequest)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMMAND_INVOKE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"run": run, "expansion": expansion, "revision": h.store.Revision()})
}

func (h pluginHandlers) handleHookReceipts(w http.ResponseWriter, r *http.Request) {
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "HOOK_RECEIPTS_UNAVAILABLE", "Plugin Hook receipts are unavailable")
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	items, next := h.runService.PluginHookReceipts(strings.TrimSpace(r.PathValue("name")), logicalID, strings.TrimSpace(r.URL.Query().Get("revisionId")), offset, limit)
	if items == nil {
		items = []hookruntime.Receipt{}
	}
	response := map[string]any{"items": items, "retention": "memory_only"}
	if next > 0 {
		response["nextCursor"] = strconv.Itoa(next)
	}
	writeJSON(w, http.StatusOK, response)
}

func (h pluginHandlers) handleHookReceiptRetry(w http.ResponseWriter, r *http.Request) {
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "HOOK_RECEIPTS_UNAVAILABLE", "Plugin Hook receipts are unavailable")
		return
	}
	receipt, err := h.runService.RetryPluginHook(r.Context(), strings.TrimSpace(r.PathValue("name")), logicalID, strings.TrimSpace(r.PathValue("receiptId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "HOOK_RETRY_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"receipt": receipt, "diagnosticOnly": true})
}

func (h pluginHandlers) handleAgentInvoke(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	var request struct {
		ExpectedDigest string `json:"expectedDigest"`
		RunID          string `json:"runId"`
		Description    string `json:"description"`
		Prompt         string `json:"prompt"`
		WorkspaceMode  string `json:"workspaceMode"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	role, err := h.store.ResolveAgentSelection(strings.TrimSpace(r.PathValue("name")), strings.TrimSpace(request.ExpectedDigest), logicalID)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if h.runService == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "AGENT_RUNTIME_UNAVAILABLE", "Subagent runtime is unavailable")
		return
	}
	description := strings.TrimSpace(request.Description)
	if description == "" {
		description = strings.TrimSpace(role.WhenToUse)
	}
	if description == "" {
		description = "Run Plugin Agent"
	}
	task, err := h.runService.SpawnSubagent(r.Context(), strings.TrimSpace(request.RunID), subagentruntime.SpawnRequest{
		Description: description, Prompt: strings.TrimSpace(request.Prompt), AgentType: role.Type, WorkspaceMode: strings.TrimSpace(request.WorkspaceMode),
	})
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "AGENT_INVOKE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task": task, "agentType": role.Type, "revision": h.store.Revision()})
}

func (h pluginHandlers) handleMCPToolState(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	logicalID, err := ir.DecodeLogicalID(strings.TrimSpace(r.PathValue("logicalId")))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_ID_INVALID", err.Error())
		return
	}
	var request struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	toolName := strings.TrimSpace(r.PathValue("toolName"))
	if h.manager != nil {
		detail, detailErr := h.store.ComponentDetail(name, logicalID)
		if detailErr != nil {
			h.writeStoreError(w, detailErr)
			return
		}
		if detail.State.Component.MCP == nil {
			writeErrorJSON(w, http.StatusBadRequest, "COMPONENT_NOT_SWITCHABLE", "component is not an MCP server")
			return
		}
		identity := plugins.ServerIdentity(name, detail.State.Component.MCP.ShortName)
		tools, toolsErr := h.manager.ServerTools(identity)
		if toolsErr == nil {
			found := false
			for _, item := range tools {
				if item.Name == toolName {
					found = true
					break
				}
			}
			if !found {
				writeErrorJSON(w, http.StatusNotFound, "MCP_TOOL_NOT_FOUND", "tool is not present in the last tools/list result")
				return
			}
		}
	}
	plugin, err := h.store.SetMCPToolEnabled(name, logicalID, toolName, request.Enabled)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	h.refreshRuntime()
	writeJSON(w, http.StatusOK, map[string]any{"plugin": h.toPluginInfo(plugin, true), "revision": h.store.Revision()})
}

func (h pluginHandlers) handleConfiguration(w http.ResponseWriter, r *http.Request) {
	if !h.ensureStore(w) {
		return
	}
	name := strings.TrimSpace(r.PathValue("name"))
	if r.Method == http.MethodGet {
		fields, err := h.store.Configuration(name)
		if err != nil {
			h.writeStoreError(w, err)
			return
		}
		if fields == nil {
			fields = []plugins.VariableField{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"fields": fields, "revision": h.store.Revision()})
		return
	}
	var request struct {
		Values map[string]any `json:"values"`
		Reset  []string       `json:"reset"`
	}
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	fields, err := h.store.UpdateConfiguration(name, request.Values, request.Reset)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if fields == nil {
		fields = []plugins.VariableField{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields, "revision": h.store.Revision()})
}
