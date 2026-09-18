# 05 — Multi-Tenancy

## Overview

Every entity in the database carries a `tenant_id` column. Tenant isolation is enforced at three layers: the HTTP middleware (resolves the tenant from the request), every repository query (always filters by `tenant_id`), and the super admin gate (allows cross-tenant access from whitelisted IPs only). White-label support is built into the `tenants` table — each tenant has a custom domain, logo, and primary color.

## Prerequisites

- Guide 04 completed (auth system working).
- `make migrate-up` has run (the `tenants` table exists with the default tenant seeded at `00000000-0000-0000-0000-000000000001`).

---

## How tenant_id flows through the system

```
Request arrives at an AUTHENTICATED route
    ↓
Authenticate middleware  (runs FIRST)
    parses JWT → extracts tid claim → sets CtxKeyTenantID
    ↓
TenantResolver middleware  (runs SECOND)
    sees CtxKeyTenantID already set → skips DB lookup entirely
    ↓
Handler
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    tenant,   _ := httpMiddleware.TenantFromCtx(r.Context())  // full entity, no extra DB call
    ↓
Repository
    WHERE tenant_id = $1  on EVERY query

Request arrives at an UNAUTHENTICATED route (login, SSO)
    ↓
TenantResolver middleware  (only middleware here)
    reads Host header → strips port → exact domain lookup in tenants table
    localhost / 127.0.0.1 / ::1 → fallback to seed default tenant (dev only)
    tenant not found → 404
    tenant.active = false → 403
    ↓
Handler (e.g. login)
    tenant is in context, proceed
```

**Key ordering rule:** In the protected route group `Authenticate` runs *before* `TenantResolver`. This ensures the "already-set-by-JWT" skip in `TenantResolver` actually fires, eliminating a redundant DB lookup on every authenticated request.

---

## Step 1 — TenantResolver middleware (implemented)

**File:** `backend/internal/adapters/http/middleware/tenant.go`

### Design decision: domain-only resolution

The middleware intentionally does **not** accept an `X-Tenant-ID` header. Any HTTP client can forge a request header — accepting it on unauthenticated routes would let an attacker target any tenant's login endpoint by simply setting the header. Domain-based resolution removes that attack surface:

- The `Host` header value comes from DNS routing / the reverse proxy, not the client.
- In production, a client cannot fake which domain they're talking to.
- API clients that need to specify a tenant (CLI tools, server-to-server) include it in the request **body**: `{"email":"…","password":"…","tenant":"my-slug"}`.

### What's implemented

```go
// Resolution order for unauthenticated requests:
// 1. CtxKeyTenantID already set (JWT ran first) → skip
// 2. Host header → exact domain match in tenants table
// 3. localhost/127.0.0.1/::1 → seed default tenant (development fallback)
func TenantResolver(db *pgxpool.Pool) func(http.Handler) http.Handler
```

Two context accessors are provided for handlers:

```go
// UUID only — used by most handlers and repository calls
tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())

// Full *Tenant entity — use when you need name, settings, storage quota, etc.
// Populated by TenantResolver; avoids a second DB round-trip.
tenant, ok := httpMiddleware.TenantFromCtx(r.Context())
```

### Host parsing

The middleware handles all Host header formats correctly:

| Host header value | Resolved host |
|-------------------|---------------|
| `example.com` | `example.com` |
| `example.com:443` | `example.com` |
| `[::1]:8080` | `[::1]` |
| `localhost:8080` | `localhost` → dev fallback |
| `127.0.0.1` | `127.0.0.1` → dev fallback |

### Error responses

| Condition | HTTP status | Body |
|-----------|-------------|------|
| No domain match (production) | 404 | `{"error":"tenant not found"}` |
| Tenant found but `active = false` | 403 | `{"error":"tenant is disabled"}` |
| DB error | 500 | `{"error":"tenant lookup failed"}` |
| localhost, DB not yet seeded | — | Request proceeds without tenant (health check works) |

---

## Step 2 — Middleware order in the router

**File:** `backend/internal/adapters/http/router.go`

```go
// Public group — no JWT yet; TenantResolver resolves from domain
r.Group(func(r chi.Router) {
    r.Use(httpMiddleware.TenantResolver(db))
    rest.MountHealth(r, db)
    rest.MountAuth(r, cfg, db, bus, log)
})

// Protected group — Authenticate FIRST, then TenantResolver
// TenantResolver sees CtxKeyTenantID set by JWT and skips DB lookup
r.Group(func(r chi.Router) {
    r.Use(httpMiddleware.Authenticate(cfg.Auth.JWTSecret))
    r.Use(httpMiddleware.TenantResolver(db))
    r.Use(httpMiddleware.ActivityTracker())

    rest.MountUsers(r, db, bus, log)
    rest.MountCollections(r, db, bus, log)
    // ...
})

// SSE — EventSource cannot set Authorization header; JWT is in ?token= query param
// TenantResolver resolves from domain; SSE handler validates JWT inline
r.Group(func(r chi.Router) {
    r.Use(httpMiddleware.TenantResolver(db))
    rest.MountSSE(r, cfg, bus, log)
})
```

---

## Step 3 — Super admin cross-tenant access

**File:** `backend/internal/adapters/http/middleware/superadmin.go` (already implemented)

A super admin's JWT contains `"super_admin"` in the roles array. `SuperAdminGate` checks:

1. Does the JWT claim the `super_admin` role?
2. Does the request come from a whitelisted IP?

Both must be true. If so, a `superAdminKey` marker is set in context and handlers can skip tenant scoping:

```go
func (h *tenantHandler) list(w http.ResponseWriter, r *http.Request) {
    if middleware.IsSuperAdmin(r.Context()) {
        // Super admin: return all tenants across the system
        tenants, total, err := h.repo.List(r.Context(), outbound.TenantFilter{Page: 1, Limit: 100})
        // ...
        return
    }
    // Regular admin: only their own tenant
    tenantID, _ := middleware.TenantIDFromCtx(r.Context())
    t, err := h.repo.GetByID(r.Context(), tenantID)
    // ...
}
```

Wire `SuperAdminGate` after `Authenticate` in the protected group:

```go
r.Group(func(r chi.Router) {
    r.Use(httpMiddleware.Authenticate(cfg.Auth.JWTSecret))
    r.Use(httpMiddleware.TenantResolver(db))
    r.Use(httpMiddleware.SuperAdminGate(cfg.SuperAdmin.AllowedIPs)) // add here
    r.Use(httpMiddleware.ActivityTracker())
    // ...
})
```

---

## Step 4 — SUPER_ADMIN_ALLOWED_IPS

Loaded exclusively from the environment (never from the database):

```dotenv
# .env — single IPs, CIDRs, or both (comma-separated)
SUPER_ADMIN_ALLOWED_IPS=127.0.0.1,::1,10.0.0.0/8,203.0.113.0/24
```

An empty list completely disables super admin login even for valid JWTs — the safe default for production until explicitly configured.

---

## Step 5 — Tenant CRUD repository (next to implement)

Create `backend/internal/adapters/persistence/postgres/tenant_repository.go`.

The `scanTenant` helper already exists in `middleware/tenant.go` — copy the column list and scan logic, or factor it into a shared `internal/adapters/persistence/postgres/scan.go` helper once the repository is wired.

```go
type tenantRepository struct{ db *pgxpool.Pool }

func NewTenantRepository(db *pgxpool.Pool) outbound.TenantRepository {
    return &tenantRepository{db: db}
}

func (r *tenantRepository) Create(ctx context.Context, t *tenant.Tenant) error {
    b, _ := json.Marshal(t.Settings)
    _, err := r.db.Exec(ctx, `
        INSERT INTO tenants
            (id, name, slug, default_language, logo, primary_color, domain, active, settings)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
        t.ID, t.Name, t.Slug, t.DefaultLanguage,
        t.Logo, t.PrimaryColor, t.Domain, t.Active, b,
    )
    return err
}

// GetByID, GetBySlug, GetByDomain, Update, Delete, List — follow the same pattern.
// Always scan: id, name, slug, default_language, logo, primary_color, domain,
//              active, settings, storage_used_bytes, storage_quota_bytes,
//              created_at, updated_at
```

---

## Step 6 — Tenant HTTP handlers (next to implement)

Create `backend/internal/adapters/http/rest/tenants.go`.

Key routes:

```
GET  /api/tenants/current   → public; returns the resolved tenant for the current domain
GET  /api/tenants           → super admin: all tenants; regular admin: own tenant only
POST /api/tenants           → super admin only; creates a new tenant
PATCH /api/tenants/{id}     → super admin or own tenant admin
DELETE /api/tenants/{id}    → super admin only
```

`GET /api/tenants/current` is the endpoint the frontend calls on load to get the tenant's name, logo, primary color, and default language:

```go
func (h *tenantHandler) current(w http.ResponseWriter, r *http.Request) {
    t, ok := httpMiddleware.TenantFromCtx(r.Context()) // no DB call needed
    if !ok {
        writeError(w, http.StatusBadRequest, "tenant not resolved")
        return
    }
    writeJSON(w, http.StatusOK, t)
}
```

---

## Step 7 — White-label configuration

The `tenants` table stores branding fields directly:

| Column | Purpose |
|--------|---------|
| `logo` | URL to tenant logo |
| `primary_color` | CSS hex, e.g. `#3B82F6` |
| `domain` | Custom domain, e.g. `cms.clientbrand.com` |
| `default_language` | BCP 47 code, e.g. `en`, `es` |
| `settings` | JSONB — `AllowRegistration`, `MaxUsers`, `Timezone`, `DateFormat` |

Apply the tenant brand at runtime in the frontend:

```typescript
// After fetching GET /api/tenants/current
const color = tenant.primary_color
if (color) document.documentElement.style.setProperty('--color-primary', color)
```

---

## Step 8 — Enforce tenant_id in every repository query

Always read the tenant from context — never accept it from the request body or URL:

```go
// ✅ correct
tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
if !ok {
    writeError(w, http.StatusUnauthorized, "tenant not resolved")
    return
}

// ❌ wrong — client-controlled
// tenantID := r.URL.Query().Get("tenant_id")
```

Every SQL query must include the tenant predicate:

```sql
-- ✅
SELECT * FROM posts WHERE tenant_id = $1 AND id = $2

-- ❌ missing tenant scope
SELECT * FROM posts WHERE id = $1
```

---

## Key Files

| File | Status | Notes |
|------|--------|-------|
| `middleware/tenant.go` | ✅ implemented | Domain-only resolution; injects full `*Tenant` |
| `middleware/auth.go` | ✅ implemented | `TenantIDFromCtx`, `Authenticate` |
| `middleware/superadmin.go` | ✅ implemented | IP/CIDR whitelist gate |
| `domain/tenant/entity.go` | ✅ implemented | `Tenant`, `Settings`, storage quota helpers |
| `ports/outbound/repositories.go` | ✅ defined | `TenantRepository` interface |
| `adapters/http/router.go` | ✅ implemented | Correct middleware ordering |
| `adapters/persistence/postgres/tenant_repository.go` | ❌ next step | Repository implementation |
| `adapters/http/rest/tenants.go` | ❌ next step | HTTP handlers |

## Next Steps

Proceed to [06 — Collection Engine](./06-collection-engine.md).
