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

// publicResourcePrefixes bypass JWT validation for the listed methods on each
// prefix, so reference data can be listed before a caller has a token — e.g.
// to populate and bootstrap a signup form — while writes and update/delete on
// the same resources still require auth. Masjids (/m) can only be listed
// anonymously; halqas (/h) can also be created anonymously since a halqa must
// exist before a signup flow has anything to attach a masjid to.
var publicResourcePrefixes = []struct {
	prefix        string
	publicMethods map[string]bool
}{
	{prefix: "/m", publicMethods: map[string]bool{http.MethodGet: true}},
	{prefix: "/h", publicMethods: map[string]bool{http.MethodGet: true, http.MethodPost: true}},
	{prefix: "/static", publicMethods: map[string]bool{http.MethodGet: true}},
}

func isPublicResource(r *http.Request) bool {
	for _, res := range publicResourcePrefixes {
		if !res.publicMethods[r.Method] {
			continue
		}

		if r.URL.Path == res.prefix || strings.HasPrefix(r.URL.Path, res.prefix+"/") {
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

// attachClaimsIfPresent parses the Authorization header, when present, and
// stashes the resulting claims in the request context. Used on public
// resources so authenticated callers get results scoped to their claims
// (e.g. m.GetAll's h_id), while unauthenticated callers still get through
// with no error even if the token is missing, malformed, or expired.
func attachClaimsIfPresent(r *http.Request) *http.Request {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return r
	}

	claims, err := jwt.Parse(token)
	if err != nil {
		return r
	}

	return r.WithContext(context.WithValue(r.Context(), ClaimsContextKey, claims))
}

func Auth() gofrHTTP.Middleware {
	return func(inner http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if publicPaths[r.URL.Path] {
				inner.ServeHTTP(w, r)
				return
			}

			if isPublicResource(r) {
				inner.ServeHTTP(w, attachClaimsIfPresent(r))
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
