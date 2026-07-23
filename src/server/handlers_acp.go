package server

import (
	"encoding/json"
	"net/http"

	"awaken/src/acp"
	awruntime "awaken/src/runtime"
)

type acpHandlers struct {
	manager    *acp.Manager
	runService *awruntime.RunService
}

type acpRunnerEnvVar struct {
	Key      string `json:"key"`
	HasValue bool   `json:"has_value"`
}

type acpRunnerView struct {
	ID                    string            `json:"id"`
	Enabled               bool              `json:"enabled"`
	Command               string            `json:"command"`
	Args                  []string          `json:"args,omitempty"`
	Env                   []acpRunnerEnvVar `json:"env,omitempty"`
	PermissionMode        string            `json:"permission_mode"`
	StartupTimeoutSeconds int               `json:"startup_timeout_seconds"`
	TurnTimeoutSeconds    int               `json:"turn_timeout_seconds"`
	MaxFrameBytes         int               `json:"max_frame_bytes"`
}

type acpRunnerPayload struct {
	ID                    string            `json:"id"`
	Enabled               *bool             `json:"enabled,omitempty"`
	Command               string            `json:"command"`
	Args                  []string          `json:"args,omitempty"`
	Env                   map[string]string `json:"env,omitempty"`
	EnvSet                map[string]string `json:"env_set,omitempty"`
	EnvRemove             []string          `json:"env_remove,omitempty"`
	PermissionMode        string            `json:"permission_mode,omitempty"`
	StartupTimeoutSeconds int               `json:"startup_timeout_seconds,omitempty"`
	TurnTimeoutSeconds    int               `json:"turn_timeout_seconds,omitempty"`
	MaxFrameBytes         int               `json:"max_frame_bytes,omitempty"`
}

func (h *acpHandlers) handleListRunners(w http.ResponseWriter, r *http.Request) {
	runners := h.manager.Registry().All()
	out := make([]acpRunnerView, 0, len(runners))
	for _, cfg := range runners {
		out = append(out, runnerView(cfg))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runners": out})
}

func (h *acpHandlers) handleCreateRunner(w http.ResponseWriter, r *http.Request) {
	var payload acpRunnerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	cfg := payload.toConfig()
	if err := h.manager.Registry().UpsertUser(cfg); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_RUNNER", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runner": runnerView(cfg)})
}

func (h *acpHandlers) handleUpdateRunner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("runner_id")
	existing, ok := h.manager.Registry().Get(id)
	if !ok {
		writeErrorJSON(w, http.StatusNotFound, "RUNNER_NOT_FOUND", id)
		return
	}
	var payload acpRunnerPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	cfg := payload.patch(existing)
	if err := h.manager.Registry().UpsertUser(cfg); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_RUNNER", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runner": runnerView(cfg)})
}

func (h *acpHandlers) handleDeleteRunner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("runner_id")
	if err := h.manager.Registry().DeleteUser(id); err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "DELETE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (h *acpHandlers) handleProbeRunner(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("runner_id")
	if err := h.manager.ProbeRunner(r.Context(), id); err != nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "PROBE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runner_id": id, "ready": true})
}

func (h *acpHandlers) handleListSessions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"sessions": h.manager.ListSessions()})
}

func (h *acpHandlers) handleCloseSession(w http.ResponseWriter, r *http.Request) {
	runnerID := r.PathValue("runner_id")
	var payload struct {
		ThreadID string `json:"thread_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload.ThreadID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "THREAD_ID_REQUIRED", "thread_id is required")
		return
	}
	if err := h.manager.CloseSession(r.Context(), payload.ThreadID, runnerID); err != nil {
		writeErrorJSON(w, http.StatusNotFound, "SESSION_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"closed": true})
}

func (h *acpHandlers) handleResolvePermission(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	interactionID := r.PathValue("interaction_id")
	var payload struct {
		OptionID  string `json:"option_id"`
		Cancelled bool   `json:"cancelled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	if err := h.manager.ResolvePermission(runID, interactionID, payload.OptionID, payload.Cancelled); err != nil {
		writeErrorJSON(w, http.StatusNotFound, "PERMISSION_NOT_FOUND", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"resolved": true})
}

func runnerView(cfg acp.RunnerConfig) acpRunnerView {
	env := make([]acpRunnerEnvVar, 0, len(cfg.Env))
	for k, v := range cfg.Env {
		env = append(env, acpRunnerEnvVar{Key: k, HasValue: v != ""})
	}
	return acpRunnerView{
		ID:                    cfg.ID,
		Enabled:               cfg.Enabled,
		Command:               cfg.Command,
		Args:                  cfg.Args,
		Env:                   env,
		PermissionMode:        string(cfg.PermissionMode),
		StartupTimeoutSeconds: cfg.StartupTimeoutSeconds,
		TurnTimeoutSeconds:    cfg.TurnTimeoutSeconds,
		MaxFrameBytes:         cfg.MaxFrameBytes,
	}
}

func (p acpRunnerPayload) toConfig() acp.RunnerConfig {
	enabled := true
	if p.Enabled != nil {
		enabled = *p.Enabled
	}
	return acp.RunnerConfig{
		ID:                    p.ID,
		Enabled:               enabled,
		Command:               p.Command,
		Args:                  p.Args,
		Env:                   p.Env,
		PermissionMode:        acp.PermissionMode(p.PermissionMode),
		StartupTimeoutSeconds: p.StartupTimeoutSeconds,
		TurnTimeoutSeconds:    p.TurnTimeoutSeconds,
		MaxFrameBytes:         p.MaxFrameBytes,
	}
}

func (p acpRunnerPayload) patch(existing acp.RunnerConfig) acp.RunnerConfig {
	cfg := existing
	if p.Command != "" {
		cfg.Command = p.Command
	}
	if p.Enabled != nil {
		cfg.Enabled = *p.Enabled
	}
	if len(p.Args) > 0 {
		cfg.Args = p.Args
	}
	if len(p.EnvSet) > 0 || len(p.EnvRemove) > 0 {
		if cfg.Env == nil {
			cfg.Env = make(map[string]string)
		}
		for k, v := range p.EnvSet {
			cfg.Env[k] = v
		}
		for _, k := range p.EnvRemove {
			delete(cfg.Env, k)
		}
	}
	if p.PermissionMode != "" {
		cfg.PermissionMode = acp.PermissionMode(p.PermissionMode)
	}
	if p.StartupTimeoutSeconds > 0 {
		cfg.StartupTimeoutSeconds = p.StartupTimeoutSeconds
	}
	if p.TurnTimeoutSeconds > 0 {
		cfg.TurnTimeoutSeconds = p.TurnTimeoutSeconds
	}
	if p.MaxFrameBytes > 0 {
		cfg.MaxFrameBytes = p.MaxFrameBytes
	}
	return cfg
}
