package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tenantdomain "github.com/imaginacolombia/cms/internal/core/domain/tenant"
)


type tenantCtxKey struct{}

// TenantResolver identifies the current tenant and injects it into the request
// context. Resolution is intentionally domain-only for unauthenticated requests:
//
//  1. Tenant already in context (set by JWT auth middleware) → skip entirely.
//     This is the normal path for every authenticated API request — the signed
//     JWT is the authoritative source of tenant identity and cannot be spoofed.
//  2. Host header → exact domain match in the tenants table.
//     The Host value is set by DNS routing / reverse proxy, not the client.
//  3. localhost / 127.0.0.1 / ::1 → fallback to the seed default tenant
//     (development only; loopback addresses never appear in production).
//
// Why no X-Tenant-ID header:
//   Any HTTP client can forge a request header. Accepting X-Tenant-ID on
//   unauthenticated routes would let an attacker target any tenant's login
//   endpoint. Domain-based resolution removes that attack surface entirely.
//   API clients that need to specify a tenant supply it in the request body
//   (e.g. {"email":"…","password":"…","tenant":"my-slug"}).
//
// Errors:
//   - 404 if no tenant can be resolved from the Host.
//   - 403 if the resolved tenant is inactive / disabled.
func TenantResolver(db *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Domain is the authoritative source of tenant identity.
			// Always resolve from the Host header — do not trust the tenant claim
			// in an incoming JWT for resolution (only for validation below).
			t, err := resolveTenant(ctx, db, r)
			if err != nil {
				writeTenantError(w, http.StatusInternalServerError, "tenant lookup failed")
				return
			}
			if t == nil {
				writeTenantError(w, http.StatusNotFound, "tenant not found")
				return
			}
			if !t.Active {
				writeTenantError(w, http.StatusForbidden, "tenant is disabled")
				return
			}

			// If Authenticate already ran (protected routes), the JWT carries a
			// tenant claim. Validate it matches the domain-resolved tenant so a
			// token issued for tenant A cannot be used on tenant B's domain.
			if jwtTenantID, ok := TenantIDFromCtx(ctx); ok && jwtTenantID != t.ID {
				writeTenantError(w, http.StatusForbidden, "token was issued for a different tenant")
				return
			}

			// Overwrite whatever Authenticate put in context with the
			// domain-authoritative values.
			ctx = context.WithValue(ctx, CtxKeyTenantID, t.ID)
			ctx = context.WithValue(ctx, tenantCtxKey{}, t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFromCtx returns the resolved Tenant for the current request.
// Available after TenantResolver has run; avoids a second DB round-trip in handlers.
func TenantFromCtx(ctx context.Context) (*tenantdomain.Tenant, bool) {
	t, ok := ctx.Value(tenantCtxKey{}).(*tenantdomain.Tenant)
	return t, ok
}

// ─── Resolution logic ─────────────────────────────────────────────────────────

func resolveTenant(ctx context.Context, db *pgxpool.Pool, r *http.Request) (*tenantdomain.Tenant, error) {
	host := cleanHost(r.Host)
	// Always resolve by domain — works for both production (real domain) and
	// development (domain = 'localhost' in the seed tenant row).
	return fetchTenantByDomain(ctx, db, host)
}

// ─── Database queries ─────────────────────────────────────────────────────────

// tenantCols is the ordered column list used in every tenant SELECT.
// Matches the scan order in scanTenant exactly.
const tenantCols = `
    id, name, slug, default_language, logo, primary_color, domain,
    active, settings, storage_used_bytes, storage_quota_bytes,
    created_at, updated_at`

func fetchTenantByID(ctx context.Context, db *pgxpool.Pool, id uuid.UUID) (*tenantdomain.Tenant, error) {
	return scanTenant(db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE id = $1 LIMIT 1`, id))
}

func fetchTenantByDomain(ctx context.Context, db *pgxpool.Pool, domain string) (*tenantdomain.Tenant, error) {
	return scanTenant(db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE domain = $1 LIMIT 1`, domain))
}

func scanTenant(row pgx.Row) (*tenantdomain.Tenant, error) {
	var t tenantdomain.Tenant
	var settingsRaw []byte

	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.DefaultLanguage,
		&t.Logo, &t.PrimaryColor, &t.Domain, &t.Active,
		&settingsRaw,
		&t.StorageUsedBytes, &t.StorageQuotaBytes,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // not found is not an error
		}
		return nil, err
	}

	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &t.Settings)
	}
	return &t, nil
}

// ─── Utilities ────────────────────────────────────────────────────────────────

// cleanHost strips the port from a Host header value.
//   "example.com:443" → "example.com"
//   "[::1]:8080"      → "[::1]"
//   "localhost"       → "localhost"
func cleanHost(host string) string {
	// IPv6 literal with port: "[::1]:8080"
	if len(host) > 0 && host[0] == '[' {
		if end := strings.Index(host, "]"); end != -1 {
			return host[:end+1]
		}
		return host
	}
	// IPv4 / hostname with port
	if i := strings.LastIndex(host, ":"); i != -1 {
		return host[:i]
	}
	return host
}

// isLocalDev returns true for loopback addresses used during development.
func isLocalDev(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	}
	return false
}

func writeTenantError(w http.ResponseWriter, status int, msg string) {
	b, _ := json.Marshal(map[string]string{"error": msg})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}
