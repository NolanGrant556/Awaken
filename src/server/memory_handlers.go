package server

import (
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"awaken/src/memory"
)

type memoryHandlers struct {
	store *memory.JSONLStore
}

type memoryListResponse struct {
	Records []memory.MemoryRecord `json:"records"`
}

type memorySearchResponse struct {
	Results []memory.SearchResult `json:"results"`
}

func (h memoryHandlers) handleListOrSearch(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeErrorJSON(w, http.StatusServiceUnavailable, "MEMORY_UNAVAILABLE", "memory store is not configured")
		return
	}

	scope := strings.TrimSpace(r.URL.Query().Get("workspace"))
	if scope == "" {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE", "workspace query parameter is required")
		return
	}
	if !filepath.IsAbs(scope) {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE", "workspace must be an absolute path")
		return
	}
	scope = filepath.Clean(scope)

	limit, err := parseMemoryLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_LIMIT", err.Error())
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		records, err := h.store.List(scope, limit)
		if err != nil {
			writeMemoryStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, memoryListResponse{Records: records})
		return
	}

	results, err := h.store.Search(scope, query, limit)
	if err != nil {
		writeMemoryStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, memorySearchResponse{Results: results})
}

func parseMemoryLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 100, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		return 0, memory.ErrInvalidLimit
	}
	return limit, nil
}

func writeMemoryStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, memory.ErrEmptyScope):
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_WORKSPACE", err.Error())
	case errors.Is(err, memory.ErrEmptyText):
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_QUERY", err.Error())
	case errors.Is(err, memory.ErrInvalidLimit):
		writeErrorJSON(w, http.StatusBadRequest, "INVALID_LIMIT", err.Error())
	default:
		writeErrorJSON(w, http.StatusInternalServerError, "INTERNAL_ERROR", "memory store failed")
	}
}
