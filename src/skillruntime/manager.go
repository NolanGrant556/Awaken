package skillruntime

import (
	"context"

	"awaken/src/skills"
	"awaken/src/tool"
)

// Manager bridges the skills.SkillRegistry with the tool registry. It
// listens for skill availability changes and registers/unregisters
// SkillTool instances under the skill__{id} namespaced name.
type Manager struct {
	registry      *skills.SkillRegistry
	inlineExec    *InlineExecutor
	logger        Logger
	registeredIDs map[string]bool // tracks currently registered tool names
}

// NewManager creates a Manager that synchronizes skill tools with the
// global tool registry.
func NewManager(reg *skills.SkillRegistry, logger Logger) *Manager {
	if logger == nil {
		logger = noopLogger{}
	}
	m := &Manager{
		registry:      reg,
		inlineExec:    &InlineExecutor{Logger: logger},
		logger:        logger,
		registeredIDs: make(map[string]bool),
	}
	// Subscribe to registry changes.
	reg.SetOnChange(m.refreshTools)
	if err := tool.RegisterToolForce(&ResourceReadTool{registry: reg}); err != nil {
		logger.Warnf("failed to register skill resource reader: %v", err)
	}
	return m
}

// RefreshTools syncs the tool registry with the current available skills.
// It unregisters tools for skills that are no longer available and
// registers tools for newly available skills.
func (m *Manager) RefreshTools() {
	m.refreshTools()
}

// refreshTools is the change callback for the skill registry.
func (m *Manager) refreshTools() {
	available := m.registry.ListAvailable()
	currentIDs := make(map[string]bool)

	for _, def := range available {
		skillID := def.ID
		toolName := SkillToolName(skillID)
		currentIDs[toolName] = true

		if m.registeredIDs[toolName] {
			continue // already registered
		}

		// Determine executor based on context mode.
		var executor SkillExecutor = m.inlineExec
		if def.Context == skills.ContextFork {
			executor = &ForkExecutor{
				Logger: m.logger,
				// StartChild is nil -> degrades to inline.
			}
		}

		argDefs := make([]SkillArgDef, len(def.Arguments))
		for i, arg := range def.Arguments {
			argDefs[i] = SkillArgDef{
				Name:        arg.Name,
				Type:        arg.Type,
				Description: arg.Description,
				Required:    arg.Required,
				Default:     arg.Default,
			}
		}

		st := &SkillTool{
			SkillID:      skillID,
			NameStr:      toolName,
			Desc:         def.Description,
			WhenToUse:    def.WhenToUse,
			Body:         def.Body,
			RootDir:      def.RootDir,
			AllowedTools: append([]string(nil), def.AllowedTools...),
			ArgDefs:      argDefs,
			Executor:     executor,
			AllowFork:    def.Context == skills.ContextFork,
		}
		if def.BodyFetcher != nil && def.McpBinding != nil {
			fetcher := def.BodyFetcher
			serverName := def.McpBinding.ServerName
			promptName := def.McpBinding.PromptName
			st.FetchBody = func(ctx context.Context) (string, error) {
				return fetcher.FetchPromptBody(ctx, serverName, promptName)
			}
		}

		if err := tool.RegisterToolForce(st); err != nil {
			m.logger.Warnf("failed to register skill tool %q: %v", toolName, err)
			continue
		}
		m.registeredIDs[toolName] = true
	}

	// Unregister tools for skills that are no longer available.
	for toolName := range m.registeredIDs {
		if !currentIDs[toolName] {
			tool.UnregisterTool(toolName)
			delete(m.registeredIDs, toolName)
		}
	}
}

// Shutdown unregisters all skill tools from the global registry.
func (m *Manager) Shutdown() {
	tool.UnregisterToolsByPrefix(SkillToolPrefix)
	tool.UnregisterTool(ResourceReadToolName)
	m.registeredIDs = make(map[string]bool)
}
