package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// APITokenAuth authenticates requests presenting a `cms_<prefix>_<secret>` bearer
// token. It must run BEFORE Authenticate/AuthenticateOptional in a group.
//
// For a non-token request it is a pass-through. For a token request it resolves
// the tenant by Host domain, looks the token up in that tenant's schema, verifies
// the secret (active or within a rotation grace window), and injects the
// api_token principal + tenant + policies so Authenticate skips the JWT and
// RequirePermission evaluates the token's policies.
func APITokenAuth(db *pgxpool.Pool, repo outbound.APITokenRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			prefix, ok := apitoken.ParsePrefix(raw)
			if !ok {
				next.ServeHTTP(w, r) // not an API token → let the JWT path handle it
				return
			}

			ctx := r.Context()
			t, err := fetchTenantByDomain(ctx, db, cleanHost(r.Host))
			if err != nil || t == nil || !t.Active {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			tok, err := repo.GetByPrefix(ctx, t.ID, prefix)
			if err != nil || tok == nil || !tok.Accepts(raw, time.Now()) {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx = context.WithValue(ctx, CtxKeyTenantID, t.ID)
			ctx = context.WithValue(ctx, tenantCtxKey{}, t)
			ctx = context.WithValue(ctx, CtxKeyPrincipal, PrincipalAPIToken)
			ctx = context.WithValue(ctx, CtxKeyTokenPolicies, tok.Policies)
			ctx = context.WithValue(ctx, CtxKeyUserEmail, "token:"+tok.Name)

			// Best-effort last-used stamp; never block the request on it.
			tenantID, tokenID := t.ID, tok.ID
			go func() {
				bg, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = repo.TouchLastUsed(bg, tenantID, tokenID, time.Now())
			}()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
