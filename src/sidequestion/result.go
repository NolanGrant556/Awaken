package sidequestion

import "strings"

func RecordResult(plan ForkPlan, outcome RunOutcome) RunResult {
	usage := mergeUsage(plan.UsageIncrement, outcome.Usage)
	result := RunResult{
		Usage:    usage,
		Provider: outcome.Provider,
	}

	if outcome.Aborted {
		result.Status = RunStatusAborted
		return result
	}

	if outcome.Err != nil {
		result.Status = RunStatusFailed
		result.Error = outcome.Err.Error()
		return result
	}

	response := strings.TrimSpace(outcome.Response)
	if response == "" {
		result.Status = RunStatusFailed
		result.Error = "no response received"
		return result
	}

	result.Status = RunStatusCompleted
	result.Response = response
	return result
}

func CompletePlan(plan ForkPlan, response string, usage UsageCounters) RunResult {
	return RecordResult(plan, RunOutcome{
		Response: response,
		Usage:    usage,
	})
}

func AbortPlan(plan ForkPlan) RunResult {
	return RecordResult(plan, RunOutcome{Aborted: true})
}

func mergeUsage(base UsageCounters, extra UsageCounters) UsageCounters {
	return UsageCounters{
		SideQuestionCountIncrement: base.SideQuestionCountIncrement + extra.SideQuestionCountIncrement,
		PromptTokens:               base.PromptTokens + extra.PromptTokens,
		CompletionTokens:           base.CompletionTokens + extra.CompletionTokens,
		TotalTokens:                base.TotalTokens + extra.TotalTokens,
	}
}
