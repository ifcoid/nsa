package middleware

import (
	"net/http"
	"strings"
)

// BearerTokenMiddleware returns an HTTP middleware that validates the
// Authorization header against a static expected token. It checks for
// "Bearer <token>" format and returns 401 if missing or invalid.
func BearerTokenMiddleware(expectedToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow CORS preflight through without auth
			if r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				sendJSONError(w, http.StatusUnauthorized, "Authorization header required")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				sendJSONError(w, http.StatusUnauthorized, "Invalid authorization header format")
				return
			}

			if parts[1] != expectedToken {
				sendJSONError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
