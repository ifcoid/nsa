package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// BearerTokenMiddleware returns an HTTP middleware that validates the
// Authorization header against a static expected token. It checks for
// "Bearer <token>" format and returns 401 if missing or invalid.
//
// Design note: the caller passes BUGLAPOR_BOT_TOKEN as the expected token,
// reusing a single secret for both Telegram API auth and MCP endpoint auth.
// This is an intentional trade-off for deployment simplicity (one env var).
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

			// Use constant-time comparison to prevent timing side-channel attacks.
			if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expectedToken)) != 1 {
				sendJSONError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
