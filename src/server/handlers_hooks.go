package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"awaken/src/hookruntime"
	awruntime "awaken/src/runtime"
)

type hookHandlers struct{ service *awruntime.RunService }

func (h hookHandlers) handleStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.service.HookStatus(strings.TrimSpace(r.URL.Query().Get("project_id")))
	if err != nil {
		writeHookError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicHookStatus(status))
}

type hookTrustRequest struct {
	ProjectID  string `json:"projectId"`
	SourcePath string `json:"sourcePath"`
	Digest     string `json:"digest,omitempty"`
}

func (h hookHandlers) handleTrust(w http.ResponseWriter, r *http.Request) {
	var request hookTrustRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "HOOK_TRUST_INVALID", err.Error())
		return
	}
	status, err := h.service.TrustProjectHook(request.ProjectID, request.SourcePath, request.Digest)
	if err != nil {
		writeHookError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicHookStatus(status))
}

func (h hookHandlers) handleRevoke(w http.ResponseWriter, r *http.Request) {
	var request hookTrustRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "HOOK_TRUST_INVALID", err.Error())
		return
	}
	status, err := h.service.RevokeProjectHook(request.ProjectID, request.SourcePath)
	if err != nil {
		writeHookError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicHookStatus(status))
}

// hookStatusResponse exposes normalized static review details so the current
// user can inspect a project config before trusting it. It never exposes
// secret references/values, environment, runtime inputs, or execution output.
type hookStatusResponse struct {
	SnapshotID       string                   `json:"snapshotId"`
	SchemaVersion    int                      `json:"schemaVersion"`
	CreatedAt        time.Time                `json:"createdAt"`
	EnabledHookCount int                      `json:"enabledHookCount"`
	Sources          []hookSourceResponse     `json:"sources"`
	Diagnostics      []hookruntime.Diagnostic `json:"diagnostics,omitempty"`
}

type hookSourceResponse struct {
	Kind        hookruntime.SourceKind   `json:"kind"`
	Path        string                   `json:"path"`
	ProjectRoot string                   `json:"projectRoot,omitempty"`
	Digest      string                   `json:"digest,omitempty"`
	Status      hookruntime.SourceStatus `json:"status"`
	HookCount   int                      `json:"hookCount"`
	Diagnostic  string                   `json:"diagnostic,omitempty"`
	Hooks       []hookruntime.ReviewHook `json:"hooks"`
}

func publicHookStatus(status hookruntime.AvailabilitySnapshot) hookStatusResponse {
	enabled := 0
	for _, hooks := range status.HooksByEvent {
		enabled += len(hooks)
	}
	sources := make([]hookSourceResponse, 0, len(status.Sources))
	for _, source := range status.Sources {
		hooks := source.ReviewHooks
		if hooks == nil {
			hooks = []hookruntime.ReviewHook{}
		}
		sources = append(sources, hookSourceResponse{
			Kind: source.Kind, Path: source.Path, ProjectRoot: source.ProjectRoot,
			Digest: source.Digest, Status: source.Status, HookCount: source.HookCount,
			Diagnostic: source.Diagnostic, Hooks: hooks,
		})
	}
	return hookStatusResponse{
		SnapshotID:       status.SnapshotID,
		SchemaVersion:    status.SchemaVersion,
		CreatedAt:        status.CreatedAt,
		EnabledHookCount: enabled,
		Sources:          sources,
		Diagnostics:      status.Diagnostics,
	}
}

func writeHookError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, awruntime.ErrProjectNotFound):
		writeErrorJSON(w, http.StatusNotFound, "PROJECT_NOT_FOUND", err.Error())
	case strings.Contains(err.Error(), "not configured"):
		writeErrorJSON(w, http.StatusServiceUnavailable, "HOOKS_UNAVAILABLE", err.Error())
	case strings.Contains(err.Error(), "source") || strings.Contains(err.Error(), "digest"):
		writeErrorJSON(w, http.StatusConflict, "HOOK_TRUST_STALE", err.Error())
	default:
		writeErrorJSON(w, http.StatusInternalServerError, "HOOKS_ERROR", err.Error())
	}
}
