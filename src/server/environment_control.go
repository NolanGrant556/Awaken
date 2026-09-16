package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"awaken/src/managedenv"
)

// These handlers are mounted exclusively under authenticated control. The
// request lifetime is one waiter; disconnecting cannot cancel other waiters.
func handlePrepareEnvironment(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	var req managedenv.Request
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeErrorJSON(w, 400, "environment_request_invalid", err.Error())
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeErrorJSON(w, 400, "environment_request_invalid", "only one request is allowed")
		return
	}
	if _, err := m.Catalog().Resolve(req); err != nil {
		writeErrorJSON(w, 400, "environment_request_invalid", err.Error())
		return
	}
	owner := managedenv.Owner{ThreadID: "desktop-environments", AgentID: "desktop", RunID: time.Now().UTC().Format(time.RFC3339Nano), Kind: "control"}
	defer m.ReleaseRun(owner.RunID)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	encoder := json.NewEncoder(w)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	emit := func(value any) {
		if err := encoder.Encode(value); err != nil {
			cancel()
			return
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	b, err := m.Ensure(ctx, req, owner, func(op managedenv.Operation) { emit(map[string]any{"type": "progress", "operation": op}) })
	if err != nil {
		code := "environment_prepare_failed"
		var typed *managedenv.Error
		if errors.As(err, &typed) {
			code = typed.Code
		}
		emit(map[string]any{"type": "error", "code": code, "message": err.Error()})
		return
	}
	emit(map[string]any{"type": "ready", "binding": b})
}

func handleInspectEnvironments(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	writeJSON(w, 200, map[string]any{"catalog": m.Catalog(), "operations": m.Operations()})
}

func handleRemoveEnvironmentComponent(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	if err := m.RemoveComponent(r.Context(), r.PathValue("component"), r.Header.Get("X-Awaken-Removal-Token")); err != nil {
		writeErrorJSON(w, 409, "environment_remove_failed", err.Error())
		return
	}
	writeJSON(w, 200, map[string]string{"status": "removed"})
}

func handleEnvironmentHostLifecycle(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	var request struct {
		Executable   string `json:"executable"`
		PID          int    `json:"pid"`
		RemovalToken string `json:"removal_token"`
	}
	var value any
	var err error
	if r.Method == "DELETE" {
		if ref := r.PathValue("reference"); ref != "" {
			err = m.ReleaseHostExecutable(ref)
		} else {
			err = m.ReleaseRemoval(r.PathValue("component"), r.Header.Get("X-Awaken-Removal-Token"))
		}
		value = map[string]bool{"released": err == nil}
	} else {
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&request); err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("only one request is allowed")
			}
		}
		if err != nil {
			writeErrorJSON(w, 400, "environment_request_invalid", err.Error())
			return
		}
		if id := r.PathValue("component"); id != "" {
			var token string
			token, err = m.ReserveRemoval(r.Context(), id, request.PID)
			value = map[string]string{"removal_token": token}
		} else {
			var ref string
			ref, err = m.HoldHostExecutable(request.Executable, request.PID, request.RemovalToken)
			value = map[string]string{"reference": ref}
		}
	}
	if err != nil {
		writeErrorJSON(w, 409, "environment_host_lifecycle_failed", err.Error())
		return
	}
	writeJSON(w, 200, value)
}

func handleInspectCLIInstallation(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	installation, err := m.InspectCLIInstallation(r.Context(), r.PathValue("component"))
	if err != nil {
		writeErrorJSON(w, 409, "environment_component_inspect_failed", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"installation": installation})
}

// The Desktop settings surface never receives executable paths or run bindings.
func handleOfficeEnvironmentStatus(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	status, err := m.OfficeBundleStatus(r.Context())
	if err != nil {
		writeErrorJSON(w, 503, "environment_status_failed", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, status)
}

func handleOfficeEnvironmentAction(w http.ResponseWriter, r *http.Request) {
	m := managedenv.Default()
	if m == nil {
		writeErrorJSON(w, 503, "environment_manager_unavailable", "共享环境管理器不可用")
		return
	}
	var req *struct {
		ExpectedInstanceID string `json:"expected_instance_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	var extra any
	if err := decoder.Decode(&req); err != nil || req == nil || decoder.Decode(&extra) != io.EOF {
		writeErrorJSON(w, 400, "environment_request_invalid", "办公环境请求无效")
		return
	}
	repair := strings.HasSuffix(r.URL.Path, "/repair")
	if repair != (req.ExpectedInstanceID != "") {
		writeErrorJSON(w, 400, "environment_request_invalid", "请刷新办公环境状态后重试")
		return
	}
	id := time.Now().UTC().Format(time.RFC3339Nano)
	owner := managedenv.Owner{ThreadID: "desktop-office-" + id, AgentID: "desktop", RunID: id, Kind: "control"}
	defer m.ReleaseControlOwner(owner)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	encoder := json.NewEncoder(w)
	emit := func(value any) {
		if encoder.Encode(value) != nil {
			cancel()
			return
		}
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
	}
	progress := func(op managedenv.Operation) { emit(map[string]any{"type": "progress", "operation": op}) }
	var err error
	if repair {
		_, err = m.RepairOfficeBundle(ctx, req.ExpectedInstanceID, owner, progress)
	} else {
		_, err = m.EnsureOfficeBundle(ctx, owner, progress)
	}
	if err != nil {
		code := "environment_prepare_failed"
		var typed *managedenv.Error
		if errors.As(err, &typed) {
			code = typed.Code
		}
		emit(map[string]any{"type": "error", "code": code, "message": err.Error()})
		return
	}
	emit(map[string]any{"type": "ready"})
}
