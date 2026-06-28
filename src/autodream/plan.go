package autodream

import "time"

func BuildActionPlan(memoryRoot string, sessionIDs []string, priorMtime time.Time, prompt string) ActionPlan {
	return ActionPlan{
		RegisterTask: RegisterDreamTaskPlan{
			SessionsReviewing: len(sessionIDs),
			PriorMtime:        priorMtime,
		},
		ForkedAgent: ForkedAgentPlan{
			QuerySource:    "auto_dream",
			ForkLabel:      "auto_dream",
			SkipTranscript: true,
			CanUseTool: ToolAccessPlan{
				Mode:       "restricted_to_memory_root",
				MemoryRoot: memoryRoot,
			},
			Prompt: prompt,
		},
		Completion: CompletionPlan{
			OnComplete: "complete dream task and keep acquired lock mtime as the consolidation stamp",
			OnFail:     "fail dream task and surface the forked-agent failure summary",
			OnRollback: "rollback consolidation lock to priorMtime so the next eligible scan can retry",
		},
		Steps: []string{
			"register dream task",
			"run forked agent with querySource auto_dream and skipTranscript true",
			"restrict canUseTool to memoryRoot",
			"complete dream task on success",
			"fail dream task and rollback lock on failure",
		},
	}
}
