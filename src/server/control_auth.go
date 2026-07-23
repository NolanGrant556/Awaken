package server

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// ControlTokenMiddleware returns an HTTP middleware that requires a valid
// Authorization: Bearer <token> header. It performs constant-time comparison
// and never reflects the token in responses or logs.
func ControlTokenMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/control/") {
				next.ServeHTTP(w, r)
				return
			}
			auth := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(auth, prefix) {
				writeControlAuthError(w)
				return
			}
			candidate := strings.TrimSpace(auth[len(prefix):])
			if candidate == "" || subtle.ConstantTimeCompare([]byte(token), []byte(candidate)) != 1 {
				writeControlAuthError(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func writeControlAuthError(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeErrorJSON(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing control token")
}
