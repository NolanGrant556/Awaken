package agentcore

import (
	stdcontext "context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"awaken/src/computer"
	"awaken/src/context"
	"awaken/src/hookruntime"
	"awaken/src/skillruntime"
	"awaken/src/skills"
	"awaken/src/subagentruntime"
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
	pendingFileComments    []SemanticFileLineComment
	preparedUserTurn       *preparedUserTurn
}

type preparedUserTurn struct {
	content        string
	attachments    []context.MessageAttachment
	composerTokens []SemanticComposerToken
	fileComments   []SemanticFileLineComment
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
	reqBuilder := NewRequestBuilder(config).BindThread(threadID)

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
	a.reqBuilder = NewRequestBuilder(config).BindThread(a.threadID)
}

// SetComputerRuntime binds a run-scoped Computer runtime to the agent. The next
// environment probe will mark ComputerEnabled=true and the runner will wire the
// ComputerExecutor into the ToolRuntime.
func (a *Agent) SetComputerRuntime(runtime *computer.Runtime) {
	if a == nil {
		return
	}
	cfg := a.config
	cfg.ComputerRuntime = runtime
	a.Reconfigure(cfg)
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
	computerEnabled := cfg.ComputerRuntime != nil && cfg.ComputerRuntime.Ready(ctx)
	a.refreshRunAvailabilityWithComputerState(ctx, reason, computerEnabled)
}

// RefreshProjectedRunAvailabilityWithReason refreshes a projection-only Agent
// from run-independent host readiness. It never creates or exposes an
// executable Computer runtime. If a real runtime is already bound, its live
// readiness remains authoritative.
func (a *Agent) RefreshProjectedRunAvailabilityWithReason(ctx stdcontext.Context, reason string, computerEnabled bool) {
	if a == nil {
		return
	}
	if runtime := a.config.ComputerRuntime; runtime != nil {
		computerEnabled = runtime.Ready(ctx)
	}
	a.refreshRunAvailabilityWithComputerState(ctx, reason, computerEnabled)
}

func (a *Agent) refreshRunAvailabilityWithComputerState(ctx stdcontext.Context, reason string, computerEnabled bool) {
	cfg := a.config
	permissionContext := permissionContextFromTaskScope(cfg.Workspace)
	hostEnvironment := tool.ResolveHostEnvironment(ctx, tool.ExecutionEnvironmentProbeOptions{
		WorkspaceRoot:     permissionContext.WorkspaceRoot,
		WorkingDirectory:  permissionContext.WorkspaceRootOrCWD(),
		SkillRoots:        localSkillRootObservations(cfg.Workspace),
		TemporaryWorkRoot: permissionContext.TemporaryWorkRoot,
		RefreshReason:     reason,
		ProbeVersions:     true,
		ComputerEnabled:   computerEnabled,
	})
	environment := hostEnvironment.Snapshot
	snapshot := tool.NewRunToolAvailabilitySnapshot(environment)
	cfg.ExecutionEnvironmentSnapshot = &environment
	cfg.ToolAvailabilitySnapshot = &snapshot
	a.Reconfigure(cfg)
}

func localSkillRootObservations(workspace WorkspaceConfig) []tool.SkillRootObservation {
	projectRoots := []string(nil)
	if !workspace.SessionWorkspace {
		projectRoots = append(projectRoots, workspace.Root)
		projectRoots = append(projectRoots, workspace.ReadableRoots...)
	}
	sourceDirs := skills.LocalSourceDirsForProjectRoots(projectRoots)
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

func (a *Agent) RegisterActivatedSkillHooks(plan *skillruntime.SkillActivationPlan) error {
	if a == nil || plan == nil || plan.HooksRaw == nil || a.config.HookDispatcher == nil {
		return nil
	}
	source := hookruntime.SourceUser
	if plan.Source == skills.SourceProject {
		source = hookruntime.SourceProject
	}
	compiled, err := hookruntime.CompileActivatedSkillHooks(plan.SkillID, plan.RootDir, plan.SkillFilePath, source, plan.HooksRaw)
	if err != nil {
		return err
	}
	return a.config.HookDispatcher.RegisterActivatedSkillHooks(compiled)
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

// SetSubagentController binds the host-owned run-scoped task registry. A new
// run must replace this value; child configs deliberately never inherit it.
func (a *Agent) SetSubagentController(controller subagentruntime.Controller) {
	if a == nil {
		return
	}
	cfg := a.config
	cfg.SubagentController = controller
	a.Reconfigure(cfg)
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

// RunNativeSubagent executes one host-registered native child with a fresh
// conversation while sharing the owning run's journal and immutable snapshots.
func (a *Agent) RunNativeSubagent(ctx stdcontext.Context, request subagentruntime.StartRequest, sink EventSink) (string, error) {
	if a == nil {
		return "", fmt.Errorf("parent Agent unavailable")
	}
	parentRunner := NewRunner(a.config, context.NewConversation(nil), NewRequestBuilder(a.config))
	parentRunner.threadID = a.threadID
	parentRunner.runID = a.runID
	parentRunner.sessionID = a.sessionID
	parentRunner.recorder = a.recorder
	return parentRunner.startNativeChild(ctx, request, sink)
}

func (a *Agent) RunStreamWithEvents(ctx stdcontext.Context, userInput string, sink EventSink) (string, error) {
	attachments, composerTokens, alreadyPersisted := a.consumeUserTurn(userInput)
	if !alreadyPersisted {
		if err := a.persistUserMessage(ctx, userInput, attachments, composerTokens, a.consumePendingFileComments(), sink); err != nil {
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
	a.reqBuilder = NewRequestBuilder(a.config).BindThread(a.threadID)
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

// PrepareUserMessage persists a composer-selected turn before pre-activation.
// RunStreamWithEvents consumes the prepared turn exactly once and will not
// append a duplicate user_message.
func (a *Agent) PrepareUserMessage(ctx stdcontext.Context, userInput string, sink EventSink) error {
	return a.PrepareUserMessageWithDisplayInput(ctx, userInput, userInput, sink)
}

// PrepareUserMessageWithDisplayInput keeps host-expanded model input out of the
// durable user_message while binding the prepared turn to the exact input the
// model will consume.
func (a *Agent) PrepareUserMessageWithDisplayInput(
	ctx stdcontext.Context,
	modelInput string,
	displayInput string,
	sink EventSink,
) error {
	if a == nil {
		return fmt.Errorf("agent unavailable")
	}
	attachments := a.consumePendingUserAttachments()
	composerTokens := a.consumePendingComposerTokens()
	fileComments := a.consumePendingFileComments()
	if err := a.persistUserMessage(ctx, displayInput, attachments, composerTokens, fileComments, sink); err != nil {
		a.preparedUserTurn = nil
		return err
	}
	a.preparedUserTurn = &preparedUserTurn{
		content: modelInput, attachments: append([]context.MessageAttachment(nil), attachments...),
		composerTokens: cloneSemanticComposerTokens(composerTokens),
		fileComments:   cloneSemanticFileLineComments(fileComments),
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

func (a *Agent) persistUserMessage(ctx stdcontext.Context, userInput string, attachments []context.MessageAttachment, composerTokens []SemanticComposerToken, fileComments []SemanticFileLineComment, sink EventSink) error {
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
	if len(fileComments) > 0 {
		payload["file_comments"] = fileComments
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

func (a *Agent) SetPendingFileComments(comments []SemanticFileLineComment) {
	a.pendingFileComments = cloneSemanticFileLineComments(comments)
}

func (a *Agent) consumePendingFileComments() []SemanticFileLineComment {
	if len(a.pendingFileComments) == 0 {
		return nil
	}
	comments := cloneSemanticFileLineComments(a.pendingFileComments)
	a.pendingFileComments = nil
	return comments
}

func cloneSemanticFileLineComments(comments []SemanticFileLineComment) []SemanticFileLineComment {
	if len(comments) == 0 {
		return nil
	}
	return append([]SemanticFileLineComment(nil), comments...)
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
