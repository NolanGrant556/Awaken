package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	awruntime "awaken/src/runtime"
)

type projectHandlers struct {
	service *awruntime.ProjectService
}

// upsertProjectRequest accepts either the single-folder form (workspace_root)
// or the multi-folder form (folders). Omitting both creates a project backed
// only by its Awaken-managed working directory.
type upsertProjectRequest struct {
	WorkspaceRoot string               `json:"workspace_root,omitempty"`
	Folders       []projectFolderInput `json:"folders,omitempty"`
	DisplayName   string               `json:"display_name,omitempty"`
}

type projectFolderInput struct {
	Path    string `json:"path"`
	Primary bool   `json:"primary,omitempty"`
}

type projectFolderRequest struct {
	Path string `json:"path"`
}

type renameProjectRequest struct {
	DisplayName string `json:"display_name"`
}

type reorderProjectsRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

type claimTrashOperationRequest struct {
	WorkerID     string `json:"worker_id,omitempty"`
	LeaseSeconds int    `json:"lease_seconds,omitempty"`
}

type failTrashOperationRequest struct {
	AttemptID string `json:"attempt_id"`
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
}

type trashAttemptRequest struct {
	AttemptID string `json:"attempt_id"`
}

func (h projectHandlers) handleListProjects(w http.ResponseWriter, r *http.Request) {
	includeRemoved, _ := strconv.ParseBool(r.URL.Query().Get("include_removed"))
	projects, err := h.service.ListProjects(includeRemoved)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h projectHandlers) handleUpsertProject(w http.ResponseWriter, r *http.Request) {
	var request upsertProjectRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	// The single-folder form stays an upsert so reopening a known folder keeps
	// reactivating its project instead of minting a duplicate.
	if len(request.Folders) == 0 && strings.TrimSpace(request.WorkspaceRoot) != "" {
		project, err := h.service.UpsertProject(request.WorkspaceRoot, request.DisplayName)
		if err != nil {
			writeProjectServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, project)
		return
	}
	folders := make([]awruntime.ProjectFolder, 0, len(request.Folders))
	for _, folder := range request.Folders {
		folders = append(folders, awruntime.ProjectFolder{
			Path:      folder.Path,
			Role:      awruntime.ProjectFolderRoleUser,
			IsPrimary: folder.Primary,
		})
	}
	project, err := h.service.CreateProject(folders, request.DisplayName)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, project)
}

func (h projectHandlers) handleRenameProject(w http.ResponseWriter, r *http.Request) {
	var request renameProjectRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	project, err := h.service.RenameProject(r.PathValue("project_id"), request.DisplayName)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h projectHandlers) handleAttachProjectFolder(w http.ResponseWriter, r *http.Request) {
	var request projectFolderRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	project, err := h.service.AttachProjectFolder(r.PathValue("project_id"), request.Path)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h projectHandlers) handleDetachProjectFolder(w http.ResponseWriter, r *http.Request) {
	var request projectFolderRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	project, err := h.service.DetachProjectFolder(r.PathValue("project_id"), request.Path)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h projectHandlers) handleSetPrimaryProjectFolder(w http.ResponseWriter, r *http.Request) {
	var request projectFolderRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	project, err := h.service.SetPrimaryProjectFolder(r.PathValue("project_id"), request.Path)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, project)
}

func (h projectHandlers) handleReorderProjects(w http.ResponseWriter, r *http.Request) {
	var request reorderProjectsRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	projects, err := h.service.ReorderProjects(request.ProjectIDs)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, projects)
}

func (h projectHandlers) handleDetachProject(w http.ResponseWriter, r *http.Request) {
	operation, err := h.service.DetachProject(r.PathValue("project_id"), idempotencyKey(r))
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleDeleteProjectThreads(w http.ResponseWriter, r *http.Request) {
	operation, err := h.service.DeleteProjectThreads(r.PathValue("project_id"), idempotencyKey(r))
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	status := http.StatusOK
	if operation.PendingCleanup > 0 {
		status = http.StatusAccepted
	}
	writeJSON(w, status, operation)
}

// handlePrepareTrash moves a single attached folder to the system trash. The
// project and its threads are untouched; project removal and history deletion
// are separate endpoints.
func (h projectHandlers) handlePrepareTrash(w http.ResponseWriter, r *http.Request) {
	var request projectFolderRequest
	if err := decodeJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.PrepareTrash(r.PathValue("project_id"), request.Path, idempotencyKey(r))
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, operation)
}

func (h projectHandlers) handleGetOperation(w http.ResponseWriter, r *http.Request) {
	operation, err := h.service.GetOperation(r.PathValue("operation_id"))
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleListPendingTrashOperations(w http.ResponseWriter, _ *http.Request) {
	operations, err := h.service.ListPendingTrashOperations()
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operations)
}

func (h projectHandlers) handleClaimTrashOperation(w http.ResponseWriter, r *http.Request) {
	request := claimTrashOperationRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	lease := time.Duration(request.LeaseSeconds) * time.Second
	operation, err := h.service.ClaimTrashOperation(r.PathValue("operation_id"), request.WorkerID, lease)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleCompleteTrashOperation(w http.ResponseWriter, r *http.Request) {
	request := trashAttemptRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.CompleteTrashOperation(r.PathValue("operation_id"), request.AttemptID)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleAttestTrashOperation(w http.ResponseWriter, r *http.Request) {
	request := trashAttemptRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.AttestTrashOperationPath(r.PathValue("operation_id"), request.AttemptID)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleBeginTrashInvocation(w http.ResponseWriter, r *http.Request) {
	request := trashAttemptRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.BeginTrashInvocation(r.PathValue("operation_id"), request.AttemptID)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleStageTrashOperation(w http.ResponseWriter, r *http.Request) {
	request := trashAttemptRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.StageTrashOperation(r.PathValue("operation_id"), request.AttemptID)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleFailTrashOperation(w http.ResponseWriter, r *http.Request) {
	request := failTrashOperationRequest{}
	if err := decodeOptionalJSON(r, &request); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	operation, err := h.service.FailTrashOperation(r.PathValue("operation_id"), request.AttemptID, request.Code, request.Message)
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h projectHandlers) handleCancelOperation(w http.ResponseWriter, r *http.Request) {
	operation, err := h.service.CancelOperation(r.PathValue("operation_id"))
	if err != nil {
		writeProjectServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func idempotencyKey(r *http.Request) string {
	return strings.TrimSpace(r.Header.Get("Idempotency-Key"))
}

func decodeOptionalJSON(r *http.Request, target any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(target)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeProjectServiceError(w http.ResponseWriter, err error) {
	code := awruntime.ProjectErrorCode(err)
	status := http.StatusInternalServerError
	switch code {
	case "PROJECT_NOT_FOUND", "PROJECT_OPERATION_NOT_FOUND":
		status = http.StatusNotFound
	case "PROJECT_FOLDER_NOT_FOUND":
		status = http.StatusNotFound
	case "PROJECT_OPERATION_CONFLICT", "PROJECT_RUNS_BUSY", "IDEMPOTENCY_KEY_CONFLICT", "PROJECT_PATH_FINGERPRINT_CHANGED", "PROJECT_TRASH_ATTEMPT_INVALID":
		status = http.StatusConflict
	case "IDEMPOTENCY_KEY_REQUIRED", "PROJECT_PATH_INVALID", "PROJECT_ORDER_INVALID", "PROJECT_NAME_INVALID":
		status = http.StatusBadRequest
	}
	if status >= 500 {
		log.Printf("[project-lifecycle] %s: %v", code, err)
	}
	writeErrorJSON(w, status, code, err.Error())
}
