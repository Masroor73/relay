package server

import "net/http"

// corsMiddleware allows the dashboard's browser origin to call the API.
// Scoped to a single configurable origin rather than a wildcard, since
// this API sits behind a shared password (ENG-35) and a wildcard origin
// combined with credentialed requests is a real security anti-pattern,
// not just unnecessary here.
func corsMiddleware(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			// Preflight requests must succeed without auth — the browser
			// sends this automatically before the real request when a
			// custom header (Authorization) is present, and it never
			// includes credentials itself.
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
