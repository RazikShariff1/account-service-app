package middleware

import (
	"context"
	"net/http"
	"strings"

	gofrHTTP "gofr.dev/pkg/gofr/http"

	"account-service/jwt"
)

type contextKey string

const ClaimsContextKey contextKey = "claims"

// publicPaths bypass JWT validation entirely.
var publicPaths = map[string]bool{
	"/login":              true,
	"/.well-known/health": true,
	"/.well-known/alive":  true,
}

func Auth() gofrHTTP.Middleware {
	return func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicPaths[r.URL.Path] {
				inner.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")

			token, ok := strings.CutPrefix(authHeader, "Bearer ")
			if !ok || token == "" {
				http.Error(w, `{"error":"missing or malformed Authorization header"}`, http.StatusUnauthorized)
				return
			}

			claims, err := jwt.Parse(token)
			if err != nil {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsContextKey, claims)
			inner.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
