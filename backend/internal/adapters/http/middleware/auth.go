package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
)

type contextKey string

const (
	CtxKeyUserID       contextKey = "user_id"
	CtxKeyTenantID     contextKey = "tenant_id"
	CtxKeyUserEmail    contextKey = "user_email"
	CtxKeyRoles        contextKey = "user_roles"
	CtxKeyIsSuperAdmin contextKey = "is_super_admin"
	// CtxKeyPrincipal holds the principal kind for the current token: "user"
	// (a system cms_users account) or "auth" (a row in a custom auth-type
	// collection, e.g. customers/operators). Empty when unauthenticated.
	CtxKeyPrincipal contextKey = "principal"
	// CtxKeyAuthCollection holds the collection name when Principal == "auth".
	CtxKeyAuthCollection contextKey = "auth_collection"
	// CtxKeyPermFilter carries the row-level filter resolved by RequirePermission
	// (from the matched Permission.Filter) so item handlers can apply it.
	CtxKeyPermFilter contextKey = "perm_filter"
	// CtxKeyTokenPolicies carries an API token's policy list when the principal is
	// an api_token, so checkPermission can evaluate it without a DB round-trip.
	CtxKeyTokenPolicies contextKey = "token_policies"
)

// Principal kinds carried in Claims.Principal / CtxKeyPrincipal.
const (
	PrincipalUser     = "user"      // a system cms_users account
	PrincipalAuth     = "auth"      // a row in a custom auth-type collection
	PrincipalAPIToken = "api_token" // a programmatic API token (policy-scoped)
)

// Claims represents the JWT payload.
type Claims struct {
	jwt.RegisteredClaims
	TenantID string   `json:"tid"`
	Email    string   `json:"email"`
	Roles    []string `json:"roles"`
	// IsSuperAdmin only ever applies to system users.
	IsSuperAdmin bool `json:"sa,omitempty"`
	// Principal is "user" or "auth" — identifies which table Subject refers to.
	Principal string `json:"principal,omitempty"`
	// Collection is the auth-type collection name when Principal == "auth".
	Collection string `json:"collection,omitempty"`
}

// Authenticate requires a valid Bearer JWT. Returns 401 if missing or invalid.
//
// Super admin override: if the JWT carries IsSuperAdmin=true AND the request
// includes an X-Tenant-ID header with a valid UUID, that UUID replaces the
// tenant_id in context. This lets the super admin panel operate in any tenant
// without re-issuing tokens.
func Authenticate(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// An API token (resolved earlier by APITokenAuth) satisfies auth on its
			// own — don't also require a JWT.
			if PrincipalFromCtx(r.Context()) == PrincipalAPIToken {
				next.ServeHTTP(w, r)
				return
			}
			claims, err := extractClaims(r, jwtSecret)
			if err != nil {
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			ctx := injectClaims(r.Context(), claims)

			// Super admin tenant override via X-Tenant-ID header.
			if claims.IsSuperAdmin {
				if hdr := r.Header.Get("X-Tenant-ID"); hdr != "" {
					if tid, err := uuid.Parse(hdr); err == nil {
						ctx = context.WithValue(ctx, CtxKeyTenantID, tid)
					}
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticateOptional parses the JWT if present but does not block unauthenticated requests.
// Used for the GraphQL endpoint which has its own field-level auth.
func AuthenticateOptional(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if claims, err := extractClaims(r, jwtSecret); err == nil {
				r = r.WithContext(injectClaims(r.Context(), claims))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ActivityTracker records last-seen timestamp (lightweight — fires async).
func ActivityTracker() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// TODO: async update users.last_active via the event bus
			next.ServeHTTP(w, r)
		})
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func extractClaims(r *http.Request, secret string) (*Claims, error) {
	raw := r.Header.Get("Authorization")
	if raw == "" {
		// Also support cookie-based auth for same-origin SPA
		if cookie, err := r.Cookie("access_token"); err == nil {
			raw = "Bearer " + cookie.Value
		}
	}

	raw = strings.TrimPrefix(raw, "Bearer ")
	if raw == "" {
		return nil, jwt.ErrTokenMalformed
	}

	claims := &Claims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	return claims, nil
}

func injectClaims(ctx context.Context, c *Claims) context.Context {
	if uid, err := uuid.Parse(c.Subject); err == nil {
		ctx = context.WithValue(ctx, CtxKeyUserID, uid)
	}
	if tid, err := uuid.Parse(c.TenantID); err == nil {
		ctx = context.WithValue(ctx, CtxKeyTenantID, tid)
	}
	ctx = context.WithValue(ctx, CtxKeyUserEmail, c.Email)
	ctx = context.WithValue(ctx, CtxKeyRoles, c.Roles)
	ctx = context.WithValue(ctx, CtxKeyIsSuperAdmin, c.IsSuperAdmin)
	principal := c.Principal
	if principal == "" {
		principal = PrincipalUser
	}
	ctx = context.WithValue(ctx, CtxKeyPrincipal, principal)
	if c.Collection != "" {
		ctx = context.WithValue(ctx, CtxKeyAuthCollection, c.Collection)
	}
	return ctx
}

// ─── Context accessors ────────────────────────────────────────────────────────

func UserIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(CtxKeyUserID).(uuid.UUID)
	return v, ok
}

func TenantIDFromCtx(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(CtxKeyTenantID).(uuid.UUID)
	return v, ok
}

func EmailFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(CtxKeyUserEmail).(string)
	return v
}

func RolesFromCtx(ctx context.Context) []string {
	v, _ := ctx.Value(CtxKeyRoles).([]string)
	return v
}

func IsSuperAdminFromCtx(ctx context.Context) bool {
	v, _ := ctx.Value(CtxKeyIsSuperAdmin).(bool)
	return v
}

// PrincipalFromCtx returns the principal kind ("user"/"auth"/"api_token"),
// defaulting to "" when unauthenticated (e.g. Public access via AuthenticateOptional).
func PrincipalFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(CtxKeyPrincipal).(string)
	return v
}

// TokenPoliciesFromCtx returns the API token's policies when the principal is an
// api_token; nil otherwise.
func TokenPoliciesFromCtx(ctx context.Context) []apitoken.Policy {
	v, _ := ctx.Value(CtxKeyTokenPolicies).([]apitoken.Policy)
	return v
}

// AuthCollectionFromCtx returns the auth-type collection name backing the
// current principal, when Principal == "auth".
func AuthCollectionFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(CtxKeyAuthCollection).(string)
	return v, ok
}

// PermFilterFromCtx returns the row-level filter attached by RequirePermission
// from the matched Permission.Filter, or nil if none applies.
func PermFilterFromCtx(ctx context.Context) any {
	return ctx.Value(CtxKeyPermFilter)
}

// WithPermFilter returns a context carrying the resolved permission filter.
func WithPermFilter(ctx context.Context, filter any) context.Context {
	return context.WithValue(ctx, CtxKeyPermFilter, filter)
}
