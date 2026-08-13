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
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxRunInvocations            = 64
	maxEventInvocations          = 16
	maxRunWall                   = 120 * time.Second
	maxAdditionalContextPerHook  = 8 * 1024
	maxAdditionalContextPerEvent = 32 * 1024
	maxAdditionalContextPerRun   = 128 * 1024
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
}

func NewDispatcher(snapshot AvailabilitySnapshot, transportPATH string) *Dispatcher {
	return &Dispatcher{snapshot: snapshot, path: transportPATH, eventInvocations: map[Event]int{}}
}

func (d *Dispatcher) SetSecretResolver(resolver SecretResolver) {
	if d != nil {
		d.secretResolver = resolver
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
			out := Outcome{HookID: hook.StableID, Event: event, SourceKind: hook.SourceKind, Status: "skipped", SpawnState: "not_started", ProcessState: "unknown", Action: "continue", ReasonCode: "hook_budget_exhausted"}
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
		out, response, elapsed := d.execute(ctx, envelope, hook, identity, recorder)
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

func (d *Dispatcher) execute(parent context.Context, envelope InputEnvelope, hook CompiledHook, id InvocationIdentity, recorder Recorder) (Outcome, Response, time.Duration) {
	started := time.Now()
	out := Outcome{InvocationID: envelope.InvocationID, HookID: hook.StableID, Event: envelope.Event, SourceKind: hook.SourceKind, Status: "failed", SpawnState: "not_started", ProcessState: "unknown", Action: "continue"}
	if hook.Command.Type == "http" {
		return d.executeHTTP(parent, envelope, hook)
	}
	if hook.Command.Type == "prompt" || hook.Command.Type == "agent" {
		if executor, ok := recorder.(ModelExecutor); ok {
			response, modelOutcome, err := executor.ExecuteModelHook(parent, envelope, hook)
			if err != nil && modelOutcome.ReasonCode == "" {
				modelOutcome.ReasonCode = "hook_model_failed"
			}
			return modelOutcome, response, time.Since(started)
		}
		out.ReasonCode = "hook_model_executor_unavailable"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	cwd, err := resolveCWD(hook, id)
	if err != nil {
		out.ReasonCode = "hook_invalid_cwd"
		return out, Response{Action: "continue"}, time.Since(started)
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(hook.Command.TimeoutMS)*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, hook.Command.Command[0], hook.Command.Command[1:]...)
	configureProcessTree(cmd)
	cmd.Dir = cwd
	cmd.Env = minimalEnvironment(d.path, envelope)
	input, _ := json.Marshal(envelope)
	cmd.Stdin = bytes.NewReader(input)
	stdout, stderr := &captureWriter{limit: hook.Command.MaxOutputBytes}, &captureWriter{limit: hook.Command.MaxOutputBytes}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err = cmd.Run()
	elapsed := time.Since(started)
	out.DurationMS = elapsed.Milliseconds()
	out.StdoutDigest = shortDigest(stdout.bytes())
	out.StderrDigest = shortDigest(stderr.bytes())
	out.OutputPreview = boundedPreview(stderr.bytes(), 512)
	if cmd.ProcessState != nil {
		out.SpawnState = "started"
		code := cmd.ProcessState.ExitCode()
		out.ExitCode = &code
	}
	if ctx.Err() != nil {
		out.Status = "timed_out"
		out.ProcessState = "timeout"
		if errors.Is(parent.Err(), context.Canceled) {
			out.Status, out.ProcessState = "cancelled", "cancelled"
		}
		out.ReasonCode = "hook_" + out.Status
		return out, Response{Action: "continue"}, elapsed
	}
	if err != nil {
		if out.SpawnState == "started" {
			out.ProcessState = "exited"
		} else {
			out.ReasonCode = "hook_spawn_failed"
		}
		if out.ReasonCode == "" {
			out.ReasonCode = "hook_nonzero_exit"
		}
		return out, Response{Action: "continue"}, elapsed
	}
	out.ProcessState = "exited"
	if stdout.overflow || stderr.overflow {
		out.ReasonCode = "hook_output_limit_exceeded"
		return out, Response{Action: "continue"}, elapsed
	}
	response, err := parseResponse(stdout.bytes(), envelope.Event)
	if err != nil {
		out.ReasonCode = "hook_protocol_invalid"
		out.OutputPreview = boundedPreview(stdout.bytes(), 512)
		return out, Response{Action: "continue"}, elapsed
	}
	out.Status, out.Action, out.Reason = "success", response.Action, response.Reason
	if isBlockingAction(response.Action) {
		out.Status = "blocked"
	}
	return out, response, elapsed
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
func minimalEnvironment(path string, envelope InputEnvelope) []string {
	values := []string{"PATH=" + path, "AWAKEN_HOOK=1", "AWAKEN_HOOK_EVENT=" + string(envelope.Event), "AWAKEN_HOOK_INVOCATION_ID=" + envelope.InvocationID}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "TMPDIR", "TEMP", "TMP", "LANG", "LC_ALL", "SystemRoot", "ComSpec"} {
		if value := os.Getenv(key); value != "" {
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
		if candidate == "*" || candidate == name {
			return true
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
