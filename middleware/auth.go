package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	gofrHTTP "gofr.dev/pkg/gofr/http"

	"main/jwt"
)

type contextKey string

const ClaimsContextKey contextKey = "claims"

// publicPaths bypass JWT validation entirely, regardless of method.
var publicPaths = map[string]bool{
	"/login":              true,
	"/signup":             true,
	"/.well-known/health": true,
	"/.well-known/alive":  true,
}

// publicResourcePrefixes bypass JWT validation for GET and POST requests, so
// reference data (halqas, masjids) can be listed and created before a caller
// has a token — e.g. to populate and bootstrap a signup form — while
// update/delete on the same resources still require auth.
var publicResourcePrefixes = []string{"/m", "/h"}

func isPublicResource(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return false
	}

	for _, prefix := range publicResourcePrefixes {
		if r.URL.Path == prefix || strings.HasPrefix(r.URL.Path, prefix+"/") {
			return true
		}
	}

	return false
}

func writeAuthError(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"error":{"message":"` + message + `"}}`))
}

func Auth() gofrHTTP.Middleware {
	return func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicPaths[r.URL.Path] || isPublicResource(r) {
				inner.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")

			token, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || token == "" {
				writeAuthError(w, "missing or malformed Authorization header")
				return
			}

			claims, err := jwt.Parse(token)
			if err != nil {
				if errors.Is(err, jwt.ErrExpiredToken) {
					writeAuthError(w, "token expired")
					return
				}

				writeAuthError(w, "invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			inner.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClaimsFromContext returns the JWT claims stashed by Auth, for handlers
// that need to scope queries to the caller's account/h_id/m_id.
func ClaimsFromContext(ctx context.Context) (*jwt.Claims, bool) {
	claims, ok := ctx.Value(ClaimsContextKey).(*jwt.Claims)
	return claims, ok
}
