package skillruntime

import (
	"awaken/src/skills"
	"awaken/src/tool"
)

// Manager owns the one model-facing skill Catalog tool plus package resource
// and install utilities. Registry changes refresh the Catalog definition; they
// never create one tool schema per Skill.
type Manager struct {
	registry    *skills.SkillRegistry
	inlineExec  *InlineExecutor
	activator   *Activator
	logger      Logger
	catalogTool *CatalogTool
}

func NewManager(reg *skills.SkillRegistry, logger Logger) *Manager {
	if logger == nil {
		logger = noopLogger{}
	}
	m := &Manager{
		registry:   reg,
		inlineExec: &InlineExecutor{Logger: logger},
		activator:  NewActivator(reg, logger),
		logger:     logger,
	}
	m.catalogTool = &CatalogTool{Registry: reg, Activator: m.activator, Executor: m.inlineExec}
	if reg != nil {
		reg.SetOnChange(m.refreshTools)
	}
	if err := tool.RegisterToolForceWithMode(&ResourceReadTool{registry: reg}, tool.DefinitionValidationStrictAll); err != nil {
		logger.Warnf("failed to register skill resource reader: %v", err)
	}
	if err := tool.RegisterToolForceWithMode(&SkillInstallTool{registry: reg}, tool.DefinitionValidationStrictBuiltin); err != nil {
		logger.Warnf("failed to register skill installer: %v", err)
	}
	m.refreshTools()
	return m
}

func (m *Manager) Activator() *Activator {
	if m == nil {
		return nil
	}
	return m.activator
}

func (m *Manager) CatalogTool() *CatalogTool {
	if m == nil {
		return nil
	}
	return m.catalogTool
}

func (m *Manager) Catalog(contextWindowTokens int) CatalogSnapshot {
	if m == nil || m.registry == nil {
		return CompileCatalog(nil, contextWindowTokens)
	}
	return CompileCatalog(m.registry.ListModelInvocable(), contextWindowTokens)
}

func (m *Manager) SetForkStarter(starter ChildAgentStarter) {
	if m == nil || m.catalogTool == nil {
		return
	}
	m.catalogTool.ForkStarter = starter
	m.refreshTools()
}

func (m *Manager) SetContextWindowProvider(provider func() int) {
	if m == nil || m.catalogTool == nil {
		return
	}
	m.catalogTool.ContextTokens = provider
	m.refreshTools()
}

func (m *Manager) SetOnActivated(fn func(plan *SkillActivationPlan)) {
	if m == nil || m.catalogTool == nil {
		return
	}
	m.catalogTool.OnActivated = fn
}

func (m *Manager) RefreshTools() { m.refreshTools() }

func (m *Manager) refreshTools() {
	if m == nil || m.catalogTool == nil {
		return
	}
	if err := tool.RegisterToolForce(m.catalogTool); err != nil {
		m.logger.Warnf("failed to register unified skill catalog tool: %v", err)
	}
}

func (m *Manager) Shutdown() {
	tool.UnregisterTool(CatalogToolName)
	tool.UnregisterTool(ResourceReadToolName)
	tool.UnregisterTool(SkillInstallToolName)
}
