package sidequestion

import (
	"errors"
	"strings"
)

var ErrEmptyQuestion = errors.New("side question is empty")

func BuildForkPlan(question string, ctx SideQuestionContext) (ForkPlan, error) {
	trimmedQuestion := strings.TrimSpace(question)
	if trimmedQuestion == "" {
		return ForkPlan{}, ErrEmptyQuestion
	}

	strippedMessages, stripped := stripInProgressAssistantMessage(ctx.Messages)
	forkMessages, boundaryIndex := messagesAfterCompactBoundary(strippedMessages)

	params, source, cacheHitExpected := selectCacheSafeParams(ctx)
	params.ToolUseContext = currentToolUseContext(ctx, params.ToolUseContext)
	params.ForkContextMessages = cloneMessages(forkMessages)

	return ForkPlan{
		Question:                    trimmedQuestion,
		CacheSafeParams:             params,
		CacheSource:                 source,
		CacheHitExpected:            cacheHitExpected,
		StrippedInProgressAssistant: stripped,
		CompactBoundaryIndex:        boundaryIndex,
		OriginalMessageCount:        len(ctx.Messages),
		ForkMessageCount:            len(forkMessages),
		UsageIncrement: UsageCounters{
			SideQuestionCountIncrement: 1,
		},
	}, nil
}

func BuildForkPlanFromRequest(req SideQuestionRequest) (ForkPlan, error) {
	return BuildForkPlan(req.Question, req.Context)
}

func stripInProgressAssistantMessage(messages []Message) ([]Message, bool) {
	if len(messages) == 0 {
		return []Message{}, false
	}

	if isInProgressAssistant(messages[len(messages)-1]) {
		return cloneMessages(messages[:len(messages)-1]), true
	}

	return cloneMessages(messages), false
}

func isInProgressAssistant(message Message) bool {
	return message.Role == RoleAssistant && message.StopReason == nil
}

func messagesAfterCompactBoundary(messages []Message) ([]Message, int) {
	boundaryIndex := -1
	for i, message := range messages {
		if message.CompactBoundary {
			boundaryIndex = i
		}
	}
	if boundaryIndex == -1 {
		return cloneMessages(messages), -1
	}
	if boundaryIndex+1 >= len(messages) {
		return []Message{}, boundaryIndex
	}
	return cloneMessages(messages[boundaryIndex+1:]), boundaryIndex
}

func selectCacheSafeParams(ctx SideQuestionContext) (CacheSafeParams, CacheSource, bool) {
	if ctx.LastCacheSafeParams != nil {
		return cloneCacheSafeParams(*ctx.LastCacheSafeParams), CacheSourceLastSafe, true
	}
	return cloneCacheSafeParams(ctx.RebuiltCacheSafeParams), CacheSourceRebuilt, false
}

func currentToolUseContext(ctx SideQuestionContext, fallback string) string {
	if strings.TrimSpace(ctx.CurrentToolUseContext) != "" {
		return ctx.CurrentToolUseContext
	}
	return fallback
}

func cloneCacheSafeParams(params CacheSafeParams) CacheSafeParams {
	params.ForkContextMessages = cloneMessages(params.ForkContextMessages)
	return params
}

func cloneMessages(messages []Message) []Message {
	if len(messages) == 0 {
		return []Message{}
	}
	cloned := make([]Message, len(messages))
	copy(cloned, messages)
	for i := range cloned {
		cloned[i].ToolCalls = append([]string(nil), messages[i].ToolCalls...)
	}
	return cloned
}
