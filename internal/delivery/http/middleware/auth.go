package middleware

import (
	"context"
	"net/http"
	"os"
	"strings"

	"aidanwoods.dev/go-paseto"
)

// SendJSONError is a local helper function for middleware to send JSON error responses
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write([]byte(`{"error":"` + message + `"}`))
}

// AuthMiddleware intercepts requests to verify PASETO v4 tokens
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Biarkan preflight CORS lewat tanpa token
		if req.Method == "OPTIONS" {
			next.ServeHTTP(w, req)
			return
		}

		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			sendJSONError(w, http.StatusUnauthorized, "Authorization header required")
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			sendJSONError(w, http.StatusUnauthorized, "Invalid authorization header format")
			return
		}

		tokenString := parts[1]

		publicKeyHex := os.Getenv("PASETO_PUBLIC_KEY")
		if publicKeyHex == "" {
			sendJSONError(w, http.StatusInternalServerError, "Server configuration error: PASETO_PUBLIC_KEY not set")
			return
		}

		publicKey, err := paseto.NewV4AsymmetricPublicKeyFromHex(publicKeyHex)
		if err != nil {
			sendJSONError(w, http.StatusInternalServerError, "Server configuration error: Invalid PASETO_PUBLIC_KEY")
			return
		}

		parser := paseto.NewParser()
		parser.AddRule(paseto.IssuedBy("agentic-slr"))

		// Verifikasi Token
		token, err := parser.ParseV4Public(publicKey, tokenString, nil)
		if err != nil {
			sendJSONError(w, http.StatusUnauthorized, "Invalid or expired token")
			return
		}

		// Ekstrak klaim
		userID, err := token.GetSubject()
		if err != nil {
			sendJSONError(w, http.StatusUnauthorized, "Invalid token subject")
			return
		}
		
		username, _ := token.GetString("username")
		role, _ := token.GetString("role")

		// Suntikkan ke Context
		ctx := req.Context()
		ctx = context.WithValue(ctx, "user_id", userID)
		ctx = context.WithValue(ctx, "username", username)
		ctx = context.WithValue(ctx, "role", role)

		// Lanjutkan dengan context yang baru
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}
