package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"awaken/src/filestore"
	"awaken/src/llm"
	"awaken/src/memory"
	awruntime "awaken/src/runtime"
)

// memoryHandlers exposes inspection plus an explicit delete operation for
// Settings. Creation, editing and superseding remain Agent-tool-only.
type memoryHandlers struct {
	projects         *awruntime.ProjectService
	defaultWorkspace string
}

type deleteMemoryRequest struct {
	ExpectedRevision int `json:"expected_revision"`
}

func (h memoryHandlers) handleDelete(w http.ResponseWriter, r *http.Request) {
	var request deleteMemoryRequest
	if err := decodeJSON(r, &request); err != nil || request.ExpectedRevision < 1 {
		writeErrorJSON(w, http.StatusBadRequest, "MEMORY_INVALID", "expected_revision must be a positive integer")
		return
	}
	workspace, err := h.workspaceForProject(r.URL.Query().Get("project_id"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT", err.Error())
		return
	}
	store, err := memory.NewStore(workspace)
	if err == nil {
		err = store.Delete(r.PathValue("memory_id"), request.ExpectedRevision)
	}
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type memorySettingsHandlers struct {
	store *memory.SettingsStore
}

func (h memorySettingsHandlers) handleGet(w http.ResponseWriter, _ *http.Request) {
	document, err := h.store.Get()
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "MEMORY_SETTINGS_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, document)
}

type updateMemorySettingsRequest struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	memory.SettingsPatch
}

func (h memorySettingsHandlers) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var request updateMemorySettingsRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "MEMORY_SETTINGS_INVALID", err.Error())
		return
	}
	document, err := h.store.Update(request.ExpectedRevision, request.SettingsPatch)
	if errors.Is(err, filestore.ErrRevisionConflict) {
		writeErrorJSON(w, http.StatusConflict, "MEMORY_SETTINGS_REVISION_CONFLICT", err.Error())
		return
	}
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "MEMORY_SETTINGS_ERROR", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, document)
}

type memoryDreamHandlers struct {
	memoryHandlers
	runner *memory.DreamRunner
}

type startMemoryDreamRequest struct {
	ProjectID    string                        `json:"project_id,omitempty"`
	Scopes       []memory.Scope                `json:"scopes,omitempty"`
	ModelProfile *awruntime.ModelProfileConfig `json:"model_profile"`
}

func (h memoryDreamHandlers) handleStart(w http.ResponseWriter, r *http.Request) {
	var request startMemoryDreamRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "DREAM_INVALID", err.Error())
		return
	}
	if request.ModelProfile == nil || strings.TrimSpace(request.ModelProfile.ModelID) == "" {
		writeErrorJSON(w, http.StatusBadRequest, "DREAM_MODEL_REQUIRED", "the current main model profile is required")
		return
	}
	for _, scope := range request.Scopes {
		if scope != memory.ScopeUser && scope != memory.ScopeProject {
			writeErrorJSON(w, http.StatusBadRequest, "DREAM_INVALID_SCOPE", fmt.Sprintf("invalid scope %q", scope))
			return
		}
	}
	workspace, err := h.workspaceForProject(request.ProjectID)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT", err.Error())
		return
	}
	store, err := memory.NewStore(workspace)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	profile := request.ModelProfile
	config := llm.Config{APIKey: profile.APIKey, BaseURL: profile.BaseURL, Model: profile.ModelID, Protocol: llm.Protocol(profile.Protocol), ProviderID: profile.ProviderID, ProfileID: profile.ProfileID, CredentialRevision: profile.CredentialRevision, Temperature: profile.Temperature, MaxTokens: profile.MaxTokens, AuthScheme: profile.AuthScheme, ContextWindow: profile.ContextWindow}
	task, err := h.runner.Start(context.Background(), store, config, request.Scopes, request.ProjectID)
	if err != nil {
		writeErrorJSON(w, http.StatusInternalServerError, "DREAM_TASK_PERSIST_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}

func (h memoryDreamHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	writeJSON(w, http.StatusOK, h.runner.List(limit, r.URL.Query().Get("project_id")))
}

func (h memoryDreamHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	task, ok := h.runner.Get(r.PathValue("task_id"))
	if !ok {
		writeErrorJSON(w, http.StatusNotFound, "DREAM_NOT_FOUND", "dream task not found")
		return
	}
	writeJSON(w, http.StatusOK, task)
}

func (h memoryDreamHandlers) handleCancel(w http.ResponseWriter, r *http.Request) {
	if !h.runner.Cancel(r.PathValue("task_id")) {
		writeErrorJSON(w, http.StatusNotFound, "DREAM_NOT_FOUND", "dream task not found or already terminal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h memoryHandlers) handleList(w http.ResponseWriter, r *http.Request) {
	workspace, err := h.workspaceForProject(r.URL.Query().Get("project_id"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT", err.Error())
		return
	}
	store, err := memory.NewStore(workspace)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	var scope *memory.Scope
	if value := strings.TrimSpace(r.URL.Query().Get("scope")); value != "" {
		parsed := memory.Scope(value)
		scope = &parsed
	}
	if scope == nil && workspace == "" {
		parsed := memory.ScopeUser
		scope = &parsed
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var types []memory.Type
	for _, value := range r.URL.Query()["type"] {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				types = append(types, memory.Type(item))
			}
		}
	}
	var statuses []memory.Status
	for _, value := range r.URL.Query()["status"] {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				statuses = append(statuses, memory.Status(item))
			}
		}
	}
	records, err := store.List(r.URL.Query().Get("query"), scope, types, statuses, limit)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	if records == nil {
		records = []memory.Record{}
	}
	writeJSON(w, http.StatusOK, records)
}

func (h memoryHandlers) handleGet(w http.ResponseWriter, r *http.Request) {
	workspace, err := h.workspaceForProject(r.URL.Query().Get("project_id"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_PROJECT", err.Error())
		return
	}
	store, err := memory.NewStore(workspace)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	record, err := store.Get(r.PathValue("memory_id"))
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (h memoryHandlers) workspaceForProject(projectID string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		if strings.TrimSpace(h.defaultWorkspace) == "" {
			return "", nil
		}
		if !filepath.IsAbs(h.defaultWorkspace) {
			return "", fmt.Errorf("configured workspace is not absolute")
		}
		return filepath.Clean(h.defaultWorkspace), nil
	}
	if h.projects == nil {
		return "", fmt.Errorf("project service unavailable")
	}
	projects, err := h.projects.ListProjects(false)
	if err != nil {
		return "", err
	}
	for _, project := range projects {
		if project.ProjectID == projectID && project.State == awruntime.ProjectStateActive {
			return project.WorkspaceRoot, nil
		}
	}
	return "", fmt.Errorf("active project %q not found", projectID)
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, memory.ErrNotFound):
		writeErrorJSON(w, http.StatusNotFound, "MEMORY_NOT_FOUND", err.Error())
	case errors.Is(err, memory.ErrRevisionConflict):
		writeErrorJSON(w, http.StatusConflict, "MEMORY_REVISION_CONFLICT", err.Error())
	case errors.Is(err, memory.ErrDuplicate):
		writeErrorJSON(w, http.StatusConflict, "MEMORY_DUPLICATE", err.Error())
	case errors.Is(err, memory.ErrSensitive):
		writeErrorJSON(w, http.StatusUnprocessableEntity, "MEMORY_SENSITIVE_CONTENT", err.Error())
	case errors.Is(err, memory.ErrMaintenanceInProgress):
		writeErrorJSON(w, http.StatusConflict, "MEMORY_MAINTENANCE_IN_PROGRESS", err.Error())
	default:
		writeErrorJSON(w, http.StatusBadRequest, "MEMORY_INVALID", err.Error())
	}
}
