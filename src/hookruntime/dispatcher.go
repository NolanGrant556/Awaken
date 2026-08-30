package hookruntime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxRunInvocations            = 64
	maxEventInvocations          = 16
	maxRunWall                   = 120 * time.Second
	maxAdditionalContextPerHook  = 8 * 1024
	maxAdditionalContextPerEvent = 32 * 1024
	maxAdditionalContextPerRun   = 128 * 1024
	maxAsyncHookTimeout          = 10 * time.Minute
)

var ErrPersistence = errors.New("hook persistence failed")

type Dispatcher struct {
	snapshot         AvailabilitySnapshot
	path             string
	mu               sync.Mutex
	runInvocations   int
	eventInvocations map[Event]int
	wall             time.Duration
	contextBytes     int
	sequence         uint64
	secretResolver   SecretResolver
	mcpToolExecutor  MCPToolExecutor
	receiptSink      ReceiptSink
}

func NewDispatcher(snapshot AvailabilitySnapshot, transportPATH string) *Dispatcher {
	return &Dispatcher{snapshot: snapshot, path: transportPATH, eventInvocations: map[Event]int{}}
}

func (d *Dispatcher) SetSecretResolver(resolver SecretResolver) {
	if d != nil {
		d.secretResolver = resolver
	}
}

func (d *Dispatcher) SetMCPToolExecutor(executor MCPToolExecutor) {
	if d != nil {
		d.mcpToolExecutor = executor
	}
}

func (d *Dispatcher) SetReceiptSink(sink ReceiptSink) {
	if d != nil {
		d.receiptSink = sink
	}
}

func (d *Dispatcher) RegisterActivatedSkillHooks(hooks []CompiledHook) error {
	if d == nil || len(hooks) == 0 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	seen := map[string]struct{}{}
	for _, existing := range d.snapshot.HooksByEvent {
		for _, hook := range existing {
			seen[hook.StableID] = struct{}{}
		}
	}
	for _, hook := range hooks {
		if _, exists := seen[hook.StableID]; exists {
			continue
		}
		seen[hook.StableID] = struct{}{}
		d.snapshot.HooksByEvent[hook.Event] = append(d.snapshot.HooksByEvent[hook.Event], hook)
	}
	return nil
}

func (d *Dispatcher) HasEvent(event Event) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.snapshot.HooksByEvent[event]) > 0
}

func (d *Dispatcher) HasHooks() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, hooks := range d.snapshot.HooksByEvent {
		if len(hooks) > 0 {
			return true
		}
	}
	return false
}

func (d *Dispatcher) Dispatch(ctx context.Context, event Event, subject map[string]any, identity InvocationIdentity, recorder Recorder) (DispatchResult, error) {
	var result DispatchResult
	d.mu.Lock()
	hooks := append([]CompiledHook(nil), d.snapshot.HooksByEvent[event]...)
	d.mu.Unlock()
	for _, hook := range hooks {
		if !matches(hook, subject) {
			continue
		}
		if err := d.reserve(event); err != nil {
			out := outcomeForHook(hook)
			out.Status, out.SpawnState, out.ProcessState, out.Action, out.ReasonCode = "skipped", "not_started", "unknown", "continue", "hook_budget_exhausted"
			result.Outcomes = append(result.Outcomes, out)
			continue
		}
		envelope := d.envelope(event, hook, subject, identity)
		if recorder == nil {
			return result, fmt.Errorf("%w: recorder unavailable", ErrPersistence)
		}
		if err := recorder.InvocationStarted(ctx, envelope, hook); err != nil {
			return result, fmt.Errorf("%w: started: %v", ErrPersistence, err)
		}
		out, response, elapsed, asyncExecution := d.execute(ctx, envelope, hook, identity, recorder, true)
		if asyncExecution != nil {
			result.Outcomes = append(result.Outcomes, out)
			d.completeAsync(envelope, hook, recorder, asyncExecution)
			continue
		}
		d.addWall(elapsed)
		if response.AdditionalContext != "" {
			if err := d.admitContext(event, response.AdditionalContext, &result); err != nil {
				out.Status, out.ReasonCode = "failed", "hook_context_limit_exceeded"
				if hook.Command.OnFailure == "block" && canBlock(event) {
					result.Action, result.Reason = blockingAction(event), out.ReasonCode
				}
			} else {
				result.AdditionalContext = append(result.AdditionalContext, response.AdditionalContext)
			}
		}
		if event == EventPreToolUse {
			if input, ok := subject["input"]; ok {
				if encoded, err := json.Marshal(input); err == nil {
					out.InputDigest = shortDigest(encoded)
				}
			}
			if response.Action == "update_input" {
				out.UpdatedInputDigest = shortDigest(response.UpdatedInput)
			}
		}
		if err := recorder.InvocationCompleted(ctx, out); err != nil {
			return result, fmt.Errorf("%w: completed: %v", ErrPersistence, err)
		}
		if d.receiptSink != nil {
			d.receiptSink.RecordHookReceipt(out)
		}
		result.Outcomes = append(result.Outcomes, out)
		if response.Action == "update_input" {
			result.Action = response.Action
			result.UpdatedInput = append(json.RawMessage(nil), response.UpdatedInput...)
		}
		if isBlockingAction(response.Action) {
			result.Action, result.Reason = response.Action, response.Reason
		}
		if out.Status != "success" && hook.Command.OnFailure == "block" && canBlock(event) {
			result.Action, result.Reason = blockingAction(event), out.ReasonCode
		}
		if isBlockingAction(result.Action) {
			break
		}
	}
	return result, nil
}

// DispatchDiagnostic executes one already compiled Plugin Hook without the
// original run subject. It is reserved for explicit retries of failed
// diagnostics and records a new receipt without replaying a prior run.
func (d *Dispatcher) DispatchDiagnostic(ctx context.Context, hook CompiledHook, identity InvocationIdentity, recorder Recorder) (Outcome, error) {
	if d == nil || hook.SourceKind != SourcePlugin || recorder == nil {
		return Outcome{}, errors.New("invalid diagnostic retry")
	}
	envelope := d.envelope(hook.Event, hook, map[string]any{}, identity)
	if err := recorder.InvocationStarted(ctx, envelope, hook); err != nil {
		return Outcome{}, err
	}
	out, _, _, asyncExecution := d.execute(ctx, envelope, hook, identity, recorder, false)
	if asyncExecution != nil {
		return Outcome{}, errors.New("diagnostic Hook unexpectedly scheduled asynchronously")
	}
	if err := recorder.InvocationCompleted(ctx, out); err != nil {
		return Outcome{}, err
	}
	if d.receiptSink != nil {
		d.receiptSink.RecordHookReceipt(out)
	}
	return out, nil
}

func (d *Dispatcher) reserve(event Event) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.runInvocations >= maxRunInvocations || d.eventInvocations[event] >= maxEventInvocations || d.wall >= maxRunWall {
		return errors.New("hook budget exhausted")
	}
	d.runInvocations++
	d.eventInvocations[event]++
	return nil
}
func (d *Dispatcher) addWall(value time.Duration) { d.mu.Lock(); d.wall += value; d.mu.Unlock() }
func (d *Dispatcher) admitContext(event Event, value string, current *DispatchResult) error {
	bytesCount := len([]byte(value))
	if bytesCount > maxAdditionalContextPerHook {
		return errors.New("per-hook context limit")
	}
	eventBytes := bytesCount
	for _, item := range current.AdditionalContext {
		eventBytes += len([]byte(item))
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if eventBytes > maxAdditionalContextPerEvent || d.contextBytes+bytesCount > maxAdditionalContextPerRun {
		return errors.New("context budget exceeded")
	}
	d.contextBytes += bytesCount
	return nil
}

func (d *Dispatcher) envelope(event Event, hook CompiledHook, subject map[string]any, id InvocationIdentity) InputEnvelope {
	d.mu.Lock()
	d.sequence++
	seq := d.sequence
	d.mu.Unlock()
	invocationID := fmt.Sprintf("hookinv_%s_%d", shortDigest([]byte(id.RunID+hook.StableID)), seq)
	return InputEnvelope{ProtocolVersion: 1, InvocationID: invocationID, HookID: hook.StableID, Event: event, Timestamp: time.Now().UTC(), ThreadID: id.ThreadID, RunID: id.RunID, TurnID: id.TurnID, SubagentTaskID: id.SubagentTaskID, Workspace: InvocationWorkspace{ProjectID: id.ProjectID, CWD: id.WorkspaceRoot, WorkspaceRoots: append([]string(nil), id.WorkspaceRoots...)}, Subject: subject}
}

func (d *Dispatcher) execute(parent context.Context, envelope InputEnvelope, hook CompiledHook, id InvocationIdentity, recorder Recorder, allowAsync bool) (Outcome, Response, time.Duration, *asyncCommandExecution) {
	started := time.Now()
	out := outcomeForHook(hook)
	out.InvocationID, out.Status, out.SpawnState, out.ProcessState, out.Action = envelope.InvocationID, "failed", "not_started", "unknown", "continue"
	if hook.Command.Type == "mcp_tool" {
		finalOut, response, elapsed := d.executeMCPTool(parent, envelope, hook, out, started)
		return finalOut, response, elapsed, nil
	}
	if hook.Command.Type == "http" {
		finalOut, response, elapsed := d.executeHTTP(parent, envelope, hook)
		return finalOut, response, elapsed, nil
	}
	if hook.Command.Type == "prompt" || hook.Command.Type == "agent" {
		if executor, ok := recorder.(ModelExecutor); ok {
			response, modelOutcome, err := executor.ExecuteModelHook(parent, envelope, hook)
			if err != nil && modelOutcome.ReasonCode == "" {
				modelOutcome.ReasonCode = "hook_model_failed"
			}
			return modelOutcome, response, time.Since(started), nil
		}
		out.ReasonCode = "hook_model_executor_unavailable"
		return out, Response{Action: "continue"}, time.Since(started), nil
	}
	cwd, err := resolveCWD(hook, id)
	if err != nil {
		out.ReasonCode = "hook_invalid_cwd"
		return out, Response{Action: "continue"}, time.Since(started), nil
	}
	if len(hook.Command.Command) == 0 {
		out.ReasonCode = "hook_spawn_failed"
		return out, Response{Action: "continue"}, time.Since(started), nil
	}
	processCtx, cancel := context.WithCancel(parent)
	timedOut := &atomic.Bool{}
	timeout := time.Duration(hook.Command.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = maxAsyncHookTimeout
	}
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		cancel()
	})
	cmd := exec.CommandContext(processCtx, hook.Command.Command[0], hook.Command.Command[1:]...)
	configureProcessTree(cmd)
	cmd.Dir = cwd
	cmd.Env = minimalEnvironment(d.path, envelope, hook)
	input, inputErr := commandInput(envelope, hook)
	if inputErr != nil {
		timer.Stop()
		cancel()
		out.ReasonCode = "hook_protocol_adapter_failed"
		return out, Response{Action: "continue"}, time.Since(started), nil
	}
	cmd.Stdin = bytes.NewReader(input)
	stdout, stderr := &captureWriter{limit: hook.Command.MaxOutputBytes}, &captureWriter{limit: hook.Command.MaxOutputBytes}
	probe := newAsyncProbeWriter(stdout)
	cmd.Stdout, cmd.Stderr = probe, stderr
	if err := cmd.Start(); err != nil {
		timer.Stop()
		cancel()
		out.ReasonCode = "hook_spawn_failed"
		return out, Response{Action: "continue"}, time.Since(started), nil
	}
	out.SpawnState, out.ProcessState = "started", "running"
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	execution := &asyncCommandExecution{
		parent: parent, processCtx: processCtx, cancel: cancel, timer: timer, timedOut: timedOut,
		cmd: cmd, wait: wait, stdout: stdout, stderr: stderr, started: started, hook: hook, envelope: envelope, outcome: out,
	}
	if allowAsync && hook.Command.Async {
		out.Status, out.Action = "scheduled", "continue"
		return out, Response{Action: "continue"}, time.Since(started), execution
	}
	if allowAsync && strings.EqualFold(strings.TrimSpace(hook.Protocol), "claude_code") {
		select {
		case declaration := <-probe.detected:
			execution.applyAsyncTimeout(declaration.TimeoutMS)
			out.Status, out.Action = "scheduled", "continue"
			return out, Response{Action: "continue"}, time.Since(started), execution
		case waitErr := <-wait:
			finalOut, response, elapsed := execution.finalize(waitErr)
			return finalOut, response, elapsed, nil
		case <-processCtx.Done():
			waitErr := <-wait
			finalOut, response, elapsed := execution.finalize(waitErr)
			return finalOut, response, elapsed, nil
		}
	}
	select {
	case waitErr := <-wait:
		finalOut, response, elapsed := execution.finalize(waitErr)
		return finalOut, response, elapsed, nil
	case <-processCtx.Done():
		waitErr := <-wait
		finalOut, response, elapsed := execution.finalize(waitErr)
		return finalOut, response, elapsed, nil
	}
}

type asyncDeclaration struct {
	TimeoutMS int
}

type asyncProbeWriter struct {
	target   *captureWriter
	pending  []byte
	detected chan asyncDeclaration
	found    bool
}

func newAsyncProbeWriter(target *captureWriter) *asyncProbeWriter {
	return &asyncProbeWriter{target: target, detected: make(chan asyncDeclaration, 1)}
}

func (w *asyncProbeWriter) Write(p []byte) (int, error) {
	n, err := w.target.Write(p)
	if w.found {
		return n, err
	}
	w.pending = append(w.pending, p...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			if len(w.pending) > 4096 {
				w.pending = append([]byte(nil), w.pending[len(w.pending)-4096:]...)
			}
			return n, err
		}
		line := bytes.TrimSpace(w.pending[:newline])
		w.pending = w.pending[newline+1:]
		var raw map[string]any
		if len(line) == 0 || json.Unmarshal(line, &raw) != nil {
			continue
		}
		async, _ := raw["async"].(bool)
		if !async {
			continue
		}
		declaration := asyncDeclaration{}
		if value, ok := raw["asyncTimeout"].(float64); ok && value > 0 {
			declaration.TimeoutMS = int(value)
		}
		w.found = true
		w.detected <- declaration
		return n, err
	}
}

type asyncCommandExecution struct {
	parent     context.Context
	processCtx context.Context
	cancel     context.CancelFunc
	timer      *time.Timer
	timedOut   *atomic.Bool
	cmd        *exec.Cmd
	wait       <-chan error
	stdout     *captureWriter
	stderr     *captureWriter
	started    time.Time
	hook       CompiledHook
	envelope   InputEnvelope
	outcome    Outcome
}

func (e *asyncCommandExecution) applyAsyncTimeout(timeoutMS int) {
	if e == nil || e.timer == nil || timeoutMS <= 0 {
		return
	}
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout > maxAsyncHookTimeout {
		timeout = maxAsyncHookTimeout
	}
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	e.timer.Reset(timeout)
}

func (e *asyncCommandExecution) finalize(waitErr error) (Outcome, Response, time.Duration) {
	elapsed := time.Since(e.started)
	out := e.outcome
	out.DurationMS = elapsed.Milliseconds()
	out.StdoutDigest = shortDigest(e.stdout.bytes())
	out.StderrDigest = shortDigest(e.stderr.bytes())
	out.OutputPreview = boundedPreview(e.stderr.bytes(), 512)
	if e.cmd.ProcessState != nil {
		code := e.cmd.ProcessState.ExitCode()
		out.ExitCode = &code
	}
	timedOut := e.timedOut.Load()
	parentCancelled := errors.Is(e.parent.Err(), context.Canceled)
	if e.timer != nil {
		e.timer.Stop()
	}
	e.cancel()
	if timedOut || parentCancelled {
		out.Status, out.ProcessState = "timed_out", "timeout"
		if parentCancelled {
			out.Status, out.ProcessState = "cancelled", "cancelled"
		}
		out.ReasonCode = "hook_" + out.Status
		return out, Response{Action: "continue"}, elapsed
	}
	out.ProcessState = "exited"
	if e.stdout.overflow || e.stderr.overflow {
		out.ReasonCode = "hook_output_limit_exceeded"
		return out, Response{Action: "continue"}, elapsed
	}
	if waitErr != nil {
		if !e.hook.Command.AsyncRewake || out.ExitCode == nil || *out.ExitCode != 2 {
			out.ReasonCode = "hook_nonzero_exit"
			return out, Response{Action: "continue"}, elapsed
		}
	}
	response, err := parseCommandResponse(e.stdout.bytes(), e.envelope.Event, e.hook.Protocol)
	if err != nil {
		out.ReasonCode = "hook_protocol_invalid"
		out.OutputPreview = boundedPreview(e.stdout.bytes(), 512)
		return out, Response{Action: "continue"}, elapsed
	}
	if e.hook.Command.AsyncRewake && strings.TrimSpace(response.AdditionalContext) == "" {
		response.AdditionalContext = firstNonEmptyString(response.Reason, boundedPreview(e.stderr.bytes(), maxAdditionalContextPerHook))
	}
	out.Status, out.Action, out.Reason = "success", response.Action, response.Reason
	if isBlockingAction(response.Action) {
		out.Status = "blocked"
	}
	return out, response, elapsed
}

func (d *Dispatcher) completeAsync(envelope InputEnvelope, hook CompiledHook, recorder Recorder, execution *asyncCommandExecution) {
	if d == nil || execution == nil {
		return
	}
	release := func() {}
	if lifecycle := d.snapshot.AsyncLifecycle; lifecycle != nil {
		if retained := lifecycle.RetainAsyncHook(); retained != nil {
			release = retained
		}
	}
	go func() {
		defer release()
		waitErr := <-execution.wait
		out, response, elapsed := execution.finalize(waitErr)
		d.addWall(elapsed)
		contextValue := strings.TrimSpace(firstNonEmptyString(response.AdditionalContext, response.Reason))
		if contextValue != "" && hook.Command.AsyncRewake && strings.TrimSpace(hook.Command.RewakeMessage) != "" {
			contextValue = strings.TrimSpace(hook.Command.RewakeMessage) + "\n\n" + contextValue
		}
		// Asynchronous handlers can never retroactively block or rewrite the
		// operation that launched them. Their feedback is delivered as untrusted
		// context on the next run of the same thread.
		out.Action = "continue"
		if out.Status == "blocked" {
			out.Status = "success"
		}
		persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := recorder.InvocationCompleted(persistCtx, out); err != nil {
			out.Status, out.ReasonCode = "failed", "hook_persistence_failed"
		}
		cancel()
		if d.receiptSink != nil {
			d.receiptSink.RecordHookReceipt(out)
		}
		if d.snapshot.AsyncSink != nil && out.Status == "success" && contextValue != "" {
			d.snapshot.AsyncSink.RecordAsyncHookCompletion(envelope.ThreadID, AsyncCompletion{
				InvocationID:      envelope.InvocationID,
				Event:             envelope.Event,
				AdditionalContext: contextValue,
				CompletedAt:       time.Now().UTC(),
			})
		}
	}()
}

func (d *Dispatcher) HasPendingAsyncContext(threadID string) bool {
	return d != nil && d.snapshot.AsyncSink != nil && d.snapshot.AsyncSink.HasAsyncHookCompletions(threadID)
}

func (d *Dispatcher) DrainPendingAsyncContext(threadID string) []AsyncCompletion {
	if d == nil || d.snapshot.AsyncSink == nil {
		return nil
	}
	return d.snapshot.AsyncSink.DrainAsyncHookCompletions(threadID)
}

func outcomeForHook(hook CompiledHook) Outcome {
	return Outcome{HookID: hook.StableID, Event: hook.Event, SourceKind: hook.SourceKind, PluginName: hook.PluginName, PluginDigest: hook.PluginDigest, LogicalID: hook.LogicalID, RevisionID: hook.RevisionID}
}

func (d *Dispatcher) executeMCPTool(parent context.Context, envelope InputEnvelope, hook CompiledHook, out Outcome, started time.Time) (Outcome, Response, time.Duration) {
	if d.mcpToolExecutor == nil {
		out.ReasonCode = "hook_mcp_executor_unavailable"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	arguments := map[string]any{}
	if len(hook.Command.Arguments) > 0 {
		if err := json.Unmarshal(hook.Command.Arguments, &arguments); err != nil {
			out.ReasonCode = "hook_mcp_arguments_invalid"
			return out, Response{Action: "continue"}, time.Since(started)
		}
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(hook.Command.TimeoutMS)*time.Millisecond)
	defer cancel()
	contextValue, err := d.mcpToolExecutor.ExecutePluginMCPTool(ctx, hook, arguments)
	elapsed := time.Since(started)
	out.DurationMS = elapsed.Milliseconds()
	if ctx.Err() != nil {
		out.Status, out.ProcessState, out.ReasonCode = "timed_out", "timeout", "hook_timed_out"
		return out, Response{Action: "continue"}, elapsed
	}
	if err != nil {
		out.ReasonCode = "hook_mcp_tool_failed"
		return out, Response{Action: "continue"}, elapsed
	}
	out.Status, out.SpawnState, out.ProcessState, out.Action = "success", "started", "completed", "continue"
	return out, Response{ProtocolVersion: 1, Action: "continue", AdditionalContext: contextValue}, elapsed
}

type captureWriter struct {
	data     bytes.Buffer
	limit    int
	overflow bool
}

func (w *captureWriter) Write(p []byte) (int, error) {
	n := len(p)
	remain := w.limit - w.data.Len()
	if remain > 0 {
		if remain > n {
			remain = n
		}
		_, _ = w.data.Write(p[:remain])
	}
	if n > remain {
		w.overflow = true
	}
	return n, nil
}
func (w *captureWriter) bytes() []byte { return w.data.Bytes() }

func parseResponse(data []byte, event Event) (Response, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return Response{ProtocolVersion: 1, Action: "continue"}, nil
	}
	var response Response
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return response, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return response, errors.New("trailing JSON")
	}
	if response.ProtocolVersion != 1 {
		return response, errors.New("unsupported protocol")
	}
	if response.Action == "" {
		response.Action = "continue"
	}
	switch response.Action {
	case "continue":
	case "block_run":
		if event != EventRunStart && event != EventUserPromptSubmit {
			return response, errors.New("invalid block_run")
		}
	case "block_tool", "update_input":
		if event != EventPreToolUse {
			return response, errors.New("invalid tool action")
		}
		if response.Action == "update_input" && len(response.UpdatedInput) == 0 {
			return response, errors.New("missing updatedInput")
		}
	case "block_compact":
		if event != EventPreCompact {
			return response, errors.New("invalid compact action")
		}
	case "block_subagent":
		if event != EventSubagentStart {
			return response, errors.New("invalid subagent action")
		}
	case "block_finish":
		if event != EventBeforeFinish {
			return response, errors.New("invalid finish action")
		}
	default:
		return response, errors.New("invalid action")
	}
	if len([]byte(response.AdditionalContext)) > maxAdditionalContextPerHook {
		return response, errors.New("context too large")
	}
	return response, nil
}

func commandInput(envelope InputEnvelope, hook CompiledHook) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(hook.Protocol)) {
	case "claude_code", "codex":
		payload := map[string]any{
			"session_id": envelope.RunID, "cwd": envelope.Workspace.CWD,
			"hook_event_name": firstNonEmptyString(hook.SourceEvent, string(envelope.Event)),
		}
		if name, ok := envelope.Subject["toolName"]; ok {
			payload["tool_name"] = name
		}
		if input, ok := envelope.Subject["input"]; ok {
			payload["tool_input"] = input
		}
		if output, ok := envelope.Subject["output"]; ok {
			payload["tool_response"] = output
		}
		if prompt, ok := envelope.Subject["text"]; ok {
			payload["prompt"] = prompt
		}
		if candidate, ok := envelope.Subject["candidateAnswer"]; ok {
			payload["reason"] = candidate
		}
		return json.Marshal(payload)
	case "cursor":
		payload := map[string]any{
			"conversation_id": envelope.ThreadID, "generation_id": envelope.RunID,
			"hook_event_name": firstNonEmptyString(hook.SourceEvent, string(envelope.Event)),
			"status":          "completed", "loop_count": 0,
		}
		for source, target := range map[string]string{"toolName": "tool_name", "input": "tool_input", "text": "prompt", "status": "status", "priorBlockCount": "loop_count"} {
			if value, ok := envelope.Subject[source]; ok {
				payload[target] = value
			}
		}
		return json.Marshal(payload)
	default:
		return json.Marshal(envelope)
	}
}

func parseCommandResponse(data []byte, event Event, protocol string) (Response, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return parseResponse(data, event)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Response{ProtocolVersion: 1, Action: "continue"}, nil
	}
	raw, err := decodeVendorHookResponse(trimmed, protocol)
	if err != nil {
		return Response{}, err
	}
	if _, canonical := raw["protocolVersion"]; canonical {
		canonical, err := json.Marshal(raw)
		if err != nil {
			return Response{}, err
		}
		return parseResponse(canonical, event)
	}
	response := Response{ProtocolVersion: 1, Action: "continue"}
	if message, _ := raw["systemMessage"].(string); strings.TrimSpace(message) != "" {
		response.AdditionalContext = message
	}
	if reason, _ := raw["reason"].(string); strings.TrimSpace(reason) != "" {
		response.Reason = reason
	}
	if decision, _ := raw["decision"].(string); strings.EqualFold(decision, "block") {
		response.Action = blockingAction(event)
	}
	if specific, ok := raw["hookSpecificOutput"].(map[string]any); ok {
		if additionalContext, _ := specific["additionalContext"].(string); strings.TrimSpace(additionalContext) != "" {
			response.AdditionalContext = additionalContext
		}
		if decision, _ := specific["permissionDecision"].(string); strings.EqualFold(decision, "deny") && event == EventPreToolUse {
			response.Action = "block_tool"
		}
		if updated, ok := specific["updatedInput"]; ok && event == EventPreToolUse {
			encoded, err := json.Marshal(updated)
			if err != nil {
				return response, err
			}
			response.Action, response.UpdatedInput = "update_input", encoded
		}
	}
	if followup, _ := raw["followup_message"].(string); strings.TrimSpace(followup) != "" {
		response.AdditionalContext = followup
		if event == EventBeforeFinish {
			response.Action = "block_finish"
		}
	}
	if len([]byte(response.AdditionalContext)) > maxAdditionalContextPerHook {
		return response, errors.New("context too large")
	}
	return response, nil
}

func decodeVendorHookResponse(data []byte, protocol string) (map[string]any, error) {
	if protocol != "claude_code" {
		var raw map[string]any
		decoder := json.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&raw); err != nil {
			return nil, err
		}
		if decoder.Decode(&struct{}{}) != io.EOF {
			return nil, errors.New("trailing JSON")
		}
		return raw, nil
	}

	sawAsyncDeclaration := false
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var raw map[string]any
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}
		if async, _ := raw["async"].(bool); async {
			sawAsyncDeclaration = true
			continue
		}
		return raw, nil
	}
	if sawAsyncDeclaration {
		return map[string]any{}, nil
	}
	return nil, errors.New("Claude Code Hook stdout has no JSON response")
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func resolveCWD(hook CompiledHook, id InvocationIdentity) (string, error) {
	cwd := id.WorkspaceRoot
	if hook.Command.CWD == "project_root" {
		cwd = hook.ProjectRoot
	}
	if cwd == "" || !filepath.IsAbs(cwd) {
		return "", errors.New("invalid cwd")
	}
	real, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", errors.New("cwd not directory")
	}
	return real, nil
}
func minimalEnvironment(path string, envelope InputEnvelope, hook CompiledHook) []string {
	values := []string{
		"PATH=" + path,
		"AWAKEN_HOOK=1",
		"AWAKEN_HOOK_EVENT=" + string(envelope.Event),
		"AWAKEN_HOOK_INVOCATION_ID=" + envelope.InvocationID,
		// Plugin packages are immutable runtime inputs. Python otherwise writes
		// __pycache__ next to imported modules and invalidates the package digest.
		"PYTHONDONTWRITEBYTECODE=1",
	}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "SystemRoot", "ComSpec"} {
		if value := os.Getenv(key); value != "" {
			values = append(values, key+"="+value)
		}
	}
	keys := make([]string, 0, len(hook.Environment))
	for key := range hook.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := hook.Environment[key]
		if key != "" && !strings.ContainsAny(key, "=\x00") && !strings.ContainsRune(value, '\x00') {
			values = append(values, key+"="+value)
		}
	}
	return values
}
func matches(hook CompiledHook, subject map[string]any) bool {
	if hook.Event != EventPreToolUse && hook.Event != EventPostToolUse && hook.Event != EventPostToolUseFailure {
		return true
	}
	name, _ := subject["toolName"].(string)
	for _, candidate := range hook.Matcher.ToolNames {
		if candidate == "*" || strings.EqualFold(candidate, name) {
			if strings.TrimSpace(hook.Matcher.InputPattern) == "" {
				return true
			}
			input, _ := subject["input"].(map[string]any)
			command, _ := input["command"].(string)
			matched, err := path.Match(hook.Matcher.InputPattern, command)
			return err == nil && matched
		}
	}
	return false
}
func canBlock(event Event) bool {
	return event == EventRunStart || event == EventUserPromptSubmit || event == EventPreToolUse || event == EventPreCompact || event == EventSubagentStart || event == EventBeforeFinish
}
func blockingAction(event Event) string {
	switch event {
	case EventPreToolUse:
		return "block_tool"
	case EventPreCompact:
		return "block_compact"
	case EventSubagentStart:
		return "block_subagent"
	case EventBeforeFinish:
		return "block_finish"
	}
	return "block_run"
}
func isBlockingAction(action string) bool {
	switch action {
	case "block_run", "block_tool", "block_compact", "block_subagent", "block_finish":
		return true
	default:
		return false
	}
}
func shortDigest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func boundedPreview(data []byte, max int) string {
	value := strings.TrimSpace(string(data))
	if len(value) > max {
		return value[:max] + "…"
	}
	return value
}
