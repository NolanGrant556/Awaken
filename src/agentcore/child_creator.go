package agentcore

import (
	"context"

	convcontext "awaken/src/context"
	"awaken/src/subagentruntime"
)

// childCreatorConfig binds construction services to this Agent's current attempt.
func (r *Runner) childCreatorConfig(sink EventSink) subagentruntime.Config {
	cfg := SubagentManagerConfig(r.config)
	cfg.ThreadID, cfg.RunID = r.threadID, r.runID
	cfg.Starter, cfg.PrepareSpawn = r.scopedChildStarter(sink), r.prepareChildSpawn(sink)
	return cfg
}

func (r *Runner) retireChildCreator(sink EventSink) {
	retirer, ok := r.subagentController.(interface{ RetireCreator(subagentruntime.Config) })
	if !ok {
		return
	}
	config := r.config
	config.RestoredContextBundleV2, config.RestoredContextWindowUsageAnchor = nil, nil
	config.CompactContextBlocks, config.HookContextBlocks, config.SkillContextBlocks = nil, nil, nil
	config.CompactedMemoryContext, config.RunPlanContextProvider, config.ToolSessionState = "", nil, nil
	config.ComputerRuntime = nil
	lean := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config).BindThread(r.threadID))
	lean.threadID, lean.runID, lean.sessionID, lean.recorder = r.threadID, r.runID, r.sessionID, r.recorder
	lean.subagentController = r.subagentController
	lean.streamLLMFunc, lean.llmRetrySleep = r.streamLLMFunc, r.llmRetrySleep
	retirer.RetireCreator(lean.childCreatorConfig(sink))
}

// BindRootCreator installs construction services for the current root attempt.
func (a *Agent) BindRootCreator(ctx context.Context, manager *subagentruntime.Manager, sink EventSink) {
	config := a.config
	config.SubagentController = manager
	r := NewRunner(config, convcontext.NewConversation(nil), NewRequestBuilder(config).BindThread(a.threadID))
	r.threadID, r.runID, r.sessionID, r.recorder = a.threadID, a.runID, a.sessionID, a.recorder
	r.subagentController = manager
	manager.Bind(ctx, "root", 0, r.childCreatorConfig(sink))
}
