package asynchooks

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"
)

type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusKilled    Status = "killed"
)

type Outcome string

const (
	OutcomeSuccess   Outcome = "success"
	OutcomeError     Outcome = "error"
	OutcomeCancelled Outcome = "cancelled"
)

type Hook struct {
	ProcessID string
	HookID    string
	HookName  string
	HookEvent string
	Command   string
	Timeout   time.Duration
	ToolName  string
	PluginID  string
	Status    Status
}

type Response struct {
	ProcessID            string
	HookID               string
	HookName             string
	HookEvent            string
	Command              string
	ToolName             string
	PluginID             string
	Stdout               string
	Stderr               string
	ExitCode             int
	Outcome              Outcome
	SyncResponse         map[string]any
	InvalidateSessionEnv bool
}

type Registry struct {
	mu    sync.RWMutex
	hooks map[string]*hookState
}

type hookState struct {
	hook      Hook
	stdout    string
	stderr    string
	exitCode  int
	delivered bool
}

func NewRegistry() *Registry {
	return &Registry{hooks: make(map[string]*hookState)}
}

func (r *Registry) Register(h Hook) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.ensureHooksLocked()
	h.Status = StatusRunning
	r.hooks[h.ProcessID] = &hookState{hook: h}
}

func (r *Registry) Complete(processID, stdout, stderr string, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	state, ok := r.hooks[processID]
	if !ok {
		return
	}
	if state.hook.Status != StatusRunning {
		return
	}
	state.hook.Status = StatusCompleted
	state.stdout = stdout
	state.stderr = stderr
	state.exitCode = exitCode
	state.delivered = false
}

func (r *Registry) Kill(processID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks == nil {
		return
	}
	state, ok := r.hooks[processID]
	if !ok || state.hook.Status != StatusRunning {
		return
	}
	delete(r.hooks, processID)
}

func (r *Registry) Pending() []Hook {
	r.mu.RLock()
	defer r.mu.RUnlock()

	hooks := make([]Hook, 0, len(r.hooks))
	for _, state := range r.hooks {
		if state.hook.Status == StatusRunning && !state.delivered {
			hooks = append(hooks, state.hook)
		}
	}
	sortHooks(hooks)
	return hooks
}

func (r *Registry) CheckResponses() []Response {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks == nil {
		return nil
	}

	responses := make([]Response, 0)
	for processID, state := range r.hooks {
		if state.delivered || state.hook.Status != StatusCompleted {
			continue
		}
		if strings.TrimSpace(state.stdout) == "" {
			delete(r.hooks, processID)
			continue
		}

		syncResponse, _ := firstSyncResponse(state.stdout)
		state.delivered = true
		responses = append(responses, state.response(syncResponse, outcomeForExitCode(state.exitCode)))
	}
	sortResponses(responses)
	return responses
}

func (r *Registry) RemoveDelivered(ids []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks == nil {
		return
	}
	for _, id := range ids {
		state, ok := r.hooks[id]
		if ok && state.delivered {
			delete(r.hooks, id)
		}
	}
}

func (r *Registry) FinalizePending() []Response {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.hooks == nil {
		return nil
	}

	responses := make([]Response, 0, len(r.hooks))
	for _, state := range r.hooks {
		if state.delivered {
			continue
		}

		switch state.hook.Status {
		case StatusCompleted:
			syncResponse, _ := firstSyncResponse(state.stdout)
			responses = append(responses, state.response(syncResponse, outcomeForExitCode(state.exitCode)))
		default:
			responses = append(responses, state.response(nil, OutcomeCancelled))
		}
	}
	r.hooks = make(map[string]*hookState)
	sortResponses(responses)
	return responses
}

func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.hooks = make(map[string]*hookState)
}

func (r *Registry) ensureHooksLocked() {
	if r.hooks == nil {
		r.hooks = make(map[string]*hookState)
	}
}

func (s *hookState) response(syncResponse map[string]any, outcome Outcome) Response {
	exitCode := s.exitCode
	if outcome == OutcomeCancelled {
		exitCode = 1
	}
	return Response{
		ProcessID:            s.hook.ProcessID,
		HookID:               s.hook.HookID,
		HookName:             s.hook.HookName,
		HookEvent:            s.hook.HookEvent,
		Command:              s.hook.Command,
		ToolName:             s.hook.ToolName,
		PluginID:             s.hook.PluginID,
		Stdout:               s.stdout,
		Stderr:               s.stderr,
		ExitCode:             exitCode,
		Outcome:              outcome,
		SyncResponse:         cloneJSONMap(syncResponse),
		InvalidateSessionEnv: s.hook.HookEvent == "SessionStart" && s.hook.Status == StatusCompleted,
	}
}

func outcomeForExitCode(exitCode int) Outcome {
	if exitCode == 0 {
		return OutcomeSuccess
	}
	return OutcomeError
}

func firstSyncResponse(stdout string) (map[string]any, bool) {
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}

		var parsed map[string]any
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}
		if _, isAsync := parsed["async"]; isAsync {
			continue
		}
		return parsed, true
	}
	return nil, false
}

func cloneJSONMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	dst := make(map[string]any, len(src))
	for key, value := range src {
		dst[key] = cloneJSONValue(value)
	}
	return dst
}

func cloneJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneJSONMap(typed)
	case []any:
		dst := make([]any, len(typed))
		for i, item := range typed {
			dst[i] = cloneJSONValue(item)
		}
		return dst
	default:
		return typed
	}
}

func sortHooks(hooks []Hook) {
	sort.Slice(hooks, func(i, j int) bool {
		return hooks[i].ProcessID < hooks[j].ProcessID
	})
}

func sortResponses(responses []Response) {
	sort.Slice(responses, func(i, j int) bool {
		return responses[i].ProcessID < responses[j].ProcessID
	})
}
