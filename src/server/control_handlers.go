package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"awaken/src/mcpclient"
	"awaken/src/networkroute"
	"awaken/src/runtime"
)

// controlHandlers serves bridge-only control routes protected by the control
// token middleware.
type controlHandlers struct {
	identity     *runtime.RuntimeIdentity
	leaseManager *runtime.LeaseManager
	runService   *runtime.RunService
	networkRoute *networkroute.Manager
	mcpManager   *mcpclient.Manager
}

func (h *controlHandlers) handleRuntimeIdentity(w http.ResponseWriter, r *http.Request) {
	if h.identity == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "RUNTIME_IDENTITY_UNAVAILABLE", "runtime identity not initialized")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"runtime_identity": h.identity.RuntimeIdentity,
		"instance_id":      h.identity.InstanceID,
		"owner_mode":       h.identity.OwnerMode,
		"base_url":         h.identity.BaseURL,
		"started_at":       h.identity.StartedAt.Format(time.RFC3339Nano),
	})
}

func (h *controlHandlers) handleCreateLease(w http.ResponseWriter, r *http.Request) {
	if h.leaseManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "LEASE_UNAVAILABLE", "lease manager not initialized")
		return
	}
	var req struct {
		OwnerKind string `json:"owner_kind"`
		OwnerID   string `json:"owner_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	if req.OwnerID == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_REQUEST", "owner_id is required")
		return
	}
	if req.OwnerKind == "" {
		req.OwnerKind = "bridge"
	}
	lease, err := h.leaseManager.CreateLease(req.OwnerKind, req.OwnerID)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "LEASE_CREATE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"lease_id":     lease.LeaseID,
		"owner_kind":   lease.OwnerKind,
		"owner_id":     lease.OwnerID,
		"expires_at":   lease.ExpiresAt.Format(time.RFC3339Nano),
		"ttl_ms":       30000,
		"heartbeat_ms": 10000,
	})
}

func (h *controlHandlers) handleHeartbeatLease(w http.ResponseWriter, r *http.Request) {
	if h.leaseManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "LEASE_UNAVAILABLE", "lease manager not initialized")
		return
	}
	leaseID := r.PathValue("lease_id")
	lease, err := h.leaseManager.Heartbeat(leaseID)
	if err != nil {
		if errors.Is(err, runtime.ErrLeaseNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "LEASE_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusGone, "LEASE_EXPIRED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"lease_id":   lease.LeaseID,
		"expires_at": lease.ExpiresAt.Format(time.RFC3339Nano),
	})
}

func (h *controlHandlers) handleReleaseLease(w http.ResponseWriter, r *http.Request) {
	if h.leaseManager == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "LEASE_UNAVAILABLE", "lease manager not initialized")
		return
	}
	leaseID := r.PathValue("lease_id")
	if err := h.leaseManager.ReleaseLease(leaseID); err != nil {
		if errors.Is(err, runtime.ErrLeaseNotFound) {
			writeErrorJSON(w, http.StatusNotFound, "LEASE_NOT_FOUND", err.Error())
			return
		}
		writeErrorJSON(w, http.StatusInternalServerError, "LEASE_RELEASE_FAILED", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "released"})
}
