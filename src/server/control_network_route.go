package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"awaken/src/networkroute"
)

const maxNetworkRouteRequestBytes = 32 * 1024

func (h *controlHandlers) handleGetNetworkRoute(w http.ResponseWriter, _ *http.Request) {
	if h.networkRoute == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "NETWORK_ROUTE_UNAVAILABLE", "network route manager not initialized")
		return
	}
	writeJSON(w, http.StatusOK, h.networkRoute.Snapshot())
}

func (h *controlHandlers) handleUpdateNetworkRoute(w http.ResponseWriter, r *http.Request) {
	if h.networkRoute == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "NETWORK_ROUTE_UNAVAILABLE", "network route manager not initialized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxNetworkRouteRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var snapshot networkroute.SystemSnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "NETWORK_ROUTE_INVALID", "invalid network route snapshot")
		return
	}
	if err := ensureJSONEOF(decoder); err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "NETWORK_ROUTE_INVALID", "network route request must contain one JSON object")
		return
	}
	previous := h.networkRoute.Snapshot()
	normalized, err := h.networkRoute.UpdateSystemSnapshot(snapshot)
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "NETWORK_ROUTE_INVALID", err.Error())
		return
	}
	if h.mcpManager != nil && normalized.Generation != previous.Generation {
		h.mcpManager.ReconnectFailedForNetworkRoute(context.Background())
	}
	writeJSON(w, http.StatusOK, normalized)
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("extra JSON value")
	}
	return err
}
