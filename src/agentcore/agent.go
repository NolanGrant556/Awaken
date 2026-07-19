package agentcore

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/context"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/tool"
)

type Agent struct {
	config                 AgentConfig
	conversation           *context.Conversation
	reqBuilder             *RequestBuilder
	threadID               string
	runID                  string
	sessionID              string
	recorder               SemanticRecorder
	pendingUserAttachments []context.MessageAttachment
	pendingComposerTokens  []SemanticComposerToken
	preparedUserTurn       *preparedUserTurn
}

type preparedUserTurn struct {
	content        string
	attachments    []context.MessageAttachment
	composerTokens []SemanticComposerToken
}

func NewAgent(config AgentConfig) *Agent {
	conv := context.NewConversation(nil)
	return NewAgentWithConversationAndThreadID(config, conv, newAwakenID("thread"))
}

func NewAgentWithThreadID(config AgentConfig, threadID string) *Agent {
	conv := context.NewConversation(nil)
	return NewAgentWithConversationAndThreadID(config, conv, threadID)
}

func NewAgentWithConversation(config AgentConfig, conv *context.Conversation) *Agent {
	return NewAgentWithConversationAndThreadID(config, conv, newAwakenID("thread"))
}

func NewAgentWithConversationAndThreadID(config AgentConfig, conv *context.Conversation, threadID string) *Agent {
	if config.ToolSessionState == nil {
		config.ToolSessionState = tool.NewToolSessionState(config.Workspace.Root)
	}
	reqBuilder := NewRequestBuilder(config)

	return &Agent{
		config:       config,
		conversation: conv,
		reqBuilder:   reqBuilder,
		threadID:     threadID,
	}
}

func (a *Agent) Reset() {
	a.conversation.Reset()
}

func (a *Agent) GetConversation() *context.Conversation {
	return a.conversation
}

func (a *Agent) GetConfig() AgentConfig {
	return a.config
}

// ModelVisibleToolPool returns the exact tool names exposed by the current
// RequestBuilder after exposure policy and any resolved Skill gate. Skill
// activation must inherit this boundary rather than the wider process-global
// registry.
func (a *Agent) ModelVisibleToolPool() []string {
	if a == nil || a.reqBuilder == nil {
		return nil
	}
	schemas := a.reqBuilder.getTools()
	names := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := strings.TrimSpace(schema.Function.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	return names
}

func (a *Agent) Reconfigure(config AgentConfig) {
	a.config = config
	a.reqBuilder = NewRequestBuilder(config)
}

// RefreshRunAvailability captures the user-machine environment and the exact
// tool catalog once for the next run. The two snapshots remain fixed until the
// next explicit refresh, even if MCP/Connector registrations change mid-run.
func (a *Agent) RefreshRunAvailability(ctx stdcontext.Context) {
	a.RefreshRunAvailabilityWithReason(ctx, "explicit_refresh")
}

// RefreshRunAvailabilityWithReason replaces the Agent configuration only at
// an explicit run/refresh boundary. Existing ToolRuntime instances retain deep
// clones of their old snapshots and cannot observe this replacement in place.
func (a *Agent) RefreshRunAvailabilityWithReason(ctx stdcontext.Context, reason string) {
	if a == nil {
		return
	}
	cfg := a.config
	permissionContext := permissionContextFromTaskScope(cfg.Workspace)
	hostEnvironment := tool.ResolveHostEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot:     permissionContext.WorkspaceRoot,
		WorkingDirectory:  permissionContext.WorkspaceRootOrCWD(),
		SkillRoots:        localSkillRootObservations(permissionContext.WorkspaceRoot),
		TemporaryWorkRoot: permissionContext.TemporaryWorkRoot,
		RefreshReason:     reason,
		ProbeVersions:     true,
	})
	environment := hostEnvironment.Snapshot
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	cfg.ExecutionEnvironmentSnapshot = &environment
	cfg.ToolAvailabilitySnapshot = &snapshot
	a.Reconfigure(cfg)
}

func localSkillRootObservations(workspaceRoot string) []tool.SkillRootObservation {
	sourceDirs := skills.LocalSourceDirs(workspaceRoot)
	result := make([]tool.SkillRootObservation, 0, len(sourceDirs)+2)
	for source, pathList := range sourceDirs {
		for _, path := range filepath.SplitList(pathList) {
			if strings.TrimSpace(path) == "" {
				continue
			}
			if !filepath.IsAbs(path) {
				if absolute, err := filepath.Abs(path); err == nil {
					path = absolute
				}
			}
			result = append(result, tool.SkillRootObservation{Source: string(source), Path: filepath.Clean(path)})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		return result[i].Path < result[j].Path
	})
	return result
}

// ApplySkillActivationPlan appends the skill's typed context block. Skill
// guidance never changes the run tool catalog.
func (a *Agent) ApplySkillActivationPlan(skillID string, effectiveTools []string, mainBody string) {
	if a == nil {
		return
	}
	cfg := a.config
	_ = effectiveTools
	if strings.TrimSpace(mainBody) != "" {
		cfg.SkillContextBlocks = append(cfg.SkillContextBlocks, ContextBlock{
			ID:        "skill_activation_" + skillID,
			SkillID:   skillID,
			InvokedAt: time.Now().UTC(),
			Content:   mainBody,
		})
	}
	a.Reconfigure(cfg)
}

// SetSemanticRecorder injects the per-thread durable journal writer.
func (a *Agent) SetSemanticRecorder(recorder SemanticRecorder) {
	a.recorder = recorder
}

// SetRunIdentity binds the current run/session identifiers for journal records.
func (a *Agent) SetRunIdentity(runID, sessionID string) {
	a.runID = runID
	a.sessionID = sessionID
}

// ExecutionIdentity returns the IDs bound by RunService for the current run.
// Custom Executor implementations that emit SemanticRecord values must use
// these IDs so restart replay can associate records with the correct run.
func (a *Agent) ExecutionIdentity() (threadID, runID, sessionID string) {
	if a == nil {
		return "", "", ""
	}
	return a.threadID, a.runID, a.sessionID
}

func (a *Agent) SemanticRecorder() SemanticRecorder {
	return a.recorder
}

// RunForkedSkill starts a true child Agent with a separate conversation while
// inheriting the current run's workspace, execution profile, tool snapshot,
// recorder, cancellation, and model credentials. Only the returned child
// result is intended to enter the parent context.
func (a *Agent) RunForkedSkill(ctx stdcontext.Context, request skillruntime.SkillForkRequest, sink EventSink) (skillruntime.SkillForkResult, error) {
	if a == nil {
		return skillruntime.SkillForkResult{}, fmt.Errorf("parent Agent unavailable")
	}
	parentRunner := NewRunner(a.config, context.NewConversation(nil), NewRequestBuilder(a.config))
	parentRunner.threadID = a.threadID
	parentRunner.runID = a.runID
	parentRunner.sessionID = a.sessionID
	parentRunner.recorder = a.recorder
	return parentRunner.startSkillChild(ctx, request, sink)
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	attachments, composerTokens, alreadyPersisted := a.consumeUserTurn(userInput)
	if !alreadyPersisted {
		if err := a.persistUserMessage(ctx, userInput, attachments, composerTokens, sink); err != nil {
			return "", err
		}
	}

	// Conversation state may only advance after the user message is durable;
	// otherwise a failed journal write leaks an unpersisted prompt into the next
	// run on the same in-memory Agent.
	a.conversation.UserInputWithAttachments(userInput, attachments)
	if a.config.MemoryRecallSetAuthority != nil {
		a.config.MemoryRecallSetAuthority(ModelCallAuthority{
			ThreadID: a.threadID, RunID: a.runID, SessionID: a.sessionID,
			Recorder: a.recorder, Sink: sink,
		})
	}
	if a.config.MemoryRecallStart != nil {
		recallCtx, cancelRecall := stdcontext.WithCancel(ctx)
		defer cancelRecall()
		a.config.MemoryRecallStart(recallCtx, userInput, recentSuccessfulToolNames(a.conversation.Context.GetMessages(), 5))
		if a.config.MemoryRecallEnd != nil {
			defer a.config.MemoryRecallEnd()
		}
	}

	runner := NewRunner(a.config, a.conversation, a.reqBuilder)
	runner.threadID = a.threadID
	runner.runID = a.runID
	runner.sessionID = a.sessionID
	runner.recorder = a.recorder

	result, err := runner.RunStreamWithEvents(ctx, sink)
	// Skill invocation and compact restoration update per-agent context. Keep
	// the runner's immutable-by-call snapshot as the next run's starting point.
	a.config = runner.config
	a.reqBuilder = NewRequestBuilder(a.config)
	return result, err
}

func recentSuccessfulToolNames(messages []context.Message, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := map[string]bool{}
	result := make([]string, 0, limit)
	for i := len(messages) - 1; i >= 0 && len(result) < limit; i-- {
		message := messages[i]
		if message.Role != context.RoleTool || message.IsError {
			continue
		}
		name := strings.TrimSpace(message.Name)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

// PrepareUserMessage persists a composer-selected Skill turn before Skill
// pre-activation. This guarantees that an activation failure never erases the
// user-authored input from durable history. RunStreamWithEvents consumes the
// prepared turn exactly once and will not append a duplicate user_message.
func (a *Agent) PrepareUserMessage(ctx stdcontext.Context, userInput string, sink EventSink) error {
	if a == nil {
		return fmt.Errorf("agent unavailable")
	}
	attachments := a.consumePendingUserAttachments()
	composerTokens := a.consumePendingComposerTokens()
	if err := a.persistUserMessage(ctx, userInput, attachments, composerTokens, sink); err != nil {
		a.preparedUserTurn = nil
		return err
	}
	a.preparedUserTurn = &preparedUserTurn{
		content: userInput, attachments: append([]context.MessageAttachment(nil), attachments...),
		composerTokens: cloneSemanticComposerTokens(composerTokens),
	}
	return nil
}

func (a *Agent) ClearPreparedUserMessage() {
	if a != nil {
		a.preparedUserTurn = nil
	}
}

func (a *Agent) consumeUserTurn(userInput string) ([]context.MessageAttachment, []SemanticComposerToken, bool) {
	if a != nil && a.preparedUserTurn != nil && a.preparedUserTurn.content == userInput {
		prepared := a.preparedUserTurn
		a.preparedUserTurn = nil
		return append([]context.MessageAttachment(nil), prepared.attachments...), cloneSemanticComposerTokens(prepared.composerTokens), true
	}
	if a != nil {
		a.preparedUserTurn = nil
	}
	return a.consumePendingUserAttachments(), a.consumePendingComposerTokens(), false
}

func (a *Agent) persistUserMessage(ctx stdcontext.Context, userInput string, attachments []context.MessageAttachment, composerTokens []SemanticComposerToken, sink EventSink) error {
	if a == nil || a.recorder == nil {
		return nil
	}
	payload := map[string]any{"content": userInput, "role": "user"}
	if len(attachments) > 0 {
		payload["attachment_count"] = len(attachments)
		payload["attachments"] = semanticUserMessageAttachments(attachments)
	}
	if len(composerTokens) > 0 {
		payload["composer_tokens"] = composerTokens
	}
	record := NewSemanticRecord(SemanticKindUserMessage, a.threadID, a.runID, "", "", a.sessionID, payload)
	if err := a.recorder.Append(ctx, record); err != nil {
		events := newRunEventEmitter(a.threadID, sink)
		events.runID = a.runID
		events.sessionID = a.sessionID
		events.recorder = a.recorder
		return events.notePersistenceFailure("user_message", err)
	}
	return nil
}

func (a *Agent) SetPendingUserAttachments(attachments []context.MessageAttachment) {
	a.pendingUserAttachments = append([]context.MessageAttachment(nil), attachments...)
}

// SetPendingComposerTokens binds the structured composer identity that must be
// written with the next durable user_message. Tokens affect runtime activation
// before Agent execution, but their display identity still belongs to the
// accepted user input and must survive Desktop restart.
func (a *Agent) SetPendingComposerTokens(tokens []SemanticComposerToken) {
	a.pendingComposerTokens = cloneSemanticComposerTokens(tokens)
}

func (a *Agent) consumePendingUserAttachments() []context.MessageAttachment {
	if len(a.pendingUserAttachments) == 0 {
		return nil
	}
	attachments := append([]context.MessageAttachment(nil), a.pendingUserAttachments...)
	a.pendingUserAttachments = nil
	return attachments
}

func (a *Agent) consumePendingComposerTokens() []SemanticComposerToken {
	if len(a.pendingComposerTokens) == 0 {
		return nil
	}
	tokens := cloneSemanticComposerTokens(a.pendingComposerTokens)
	a.pendingComposerTokens = nil
	return tokens
}

func semanticUserMessageAttachments(attachments []context.MessageAttachment) []SemanticUserMessageAttachment {
	if len(attachments) == 0 {
		return nil
	}
	result := make([]SemanticUserMessageAttachment, 0, len(attachments))
	for _, attachment := range attachments {
		result = append(result, SemanticUserMessageAttachment{
			ID:          attachment.ID,
			Kind:        attachment.Kind,
			MimeType:    attachment.MimeType,
			Path:        attachment.Path,
			DisplayName: attachment.DisplayName,
			Width:       attachment.Width,
			Height:      attachment.Height,
			ByteSize:    attachment.ByteSize,
			StorageKey:  attachment.StorageKey,
		})
	}
	return result
}

func cloneSemanticComposerTokens(tokens []SemanticComposerToken) []SemanticComposerToken {
	if len(tokens) == 0 {
		return nil
	}
	cloned := make([]SemanticComposerToken, len(tokens))
	for i, token := range tokens {
		cloned[i] = token
		if token.Payload == nil {
			continue
		}
		cloned[i].Payload = make(map[string]any, len(token.Payload))
		for key, value := range token.Payload {
			cloned[i].Payload[key] = value
		}
	}
	return cloned
}
