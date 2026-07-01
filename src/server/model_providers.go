package server

import (
	"encoding/json"
	"net/http"

	"awaken/src/modelprofile"
)

func handleModelProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(modelprofile.BuiltinProviders()); err != nil {
		http.Error(w, "failed to encode model providers", http.StatusInternalServerError)
	}
}
