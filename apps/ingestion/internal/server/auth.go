package server

import (
	"crypto/subtle"
	"net/http"
)

// dashboardAuthMiddleware gates access behind a single shared password via
// HTTP Basic Auth, per ARCHITECTURE.md §10 — deliberately lightweight,
// appropriate for a single-operator public portfolio deployment rather
// than multi-user access control.
func dashboardAuthMiddleware(password string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, providedPassword, ok := r.BasicAuth()

		// subtle.ConstantTimeCompare requires equal-length inputs to be
		// meaningful; mismatched lengths are handled by the length check
		// itself, which does not leak timing information proportional to
		// how many characters matched.
		if !ok || len(providedPassword) != len(password) ||
			subtle.ConstantTimeCompare([]byte(providedPassword), []byte(password)) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="dashboard"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next(w, r)
	}
}
