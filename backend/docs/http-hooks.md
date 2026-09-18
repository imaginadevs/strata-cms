# HTTP hooks

The `internal/core/hooks` package lets a plugin or integration register a
custom HTTP endpoint that owns its full request/response cycle — used for
things a plain collection CRUD verb can't express: payment provider webhooks
(Wompi, Stripe, …), outbound integration callbacks, one-off admin actions,
etc.

This is different from [collection hooks](collection-hooks.md), which run
*around* an existing `/api/items/{collection}` create/update/delete call.

## Registering a route

`hooks.RegisterRoute(method, path, tenantScope, handler)` — `tenantScope` is
`nil` for all tenants, or `&tenantUUID` for a single-tenant route.

```go
package providers

import (
    "net/http"

    "github.com/imaginacolombia/cms/internal/core/hooks"
)

func init() {
    // nil scope = any tenant Host; pass &tenantUUID for a tenant-only route.
    hooks.RegisterRoute(http.MethodPost, "/wompi", nil, func(w http.ResponseWriter, r *http.Request, hc hooks.HookContext) {
        // 1. Validate the webhook signature (provider-specific secret/header).
        // 2. Use hc.TenantID + hc.Repos to look up/update tenant data.
        // 3. Respond — most webhook providers expect a 200/204 with no body.
        w.WriteHeader(http.StatusNoContent)
    })
}
```

`internal/core/hooks/providers/stub.go` is a minimal working example
(`POST /api/hooks/example` → `204 No Content`) — copy it as a starting point
for a real provider.

The route is mounted at `/api/hooks<path>`, so `"/wompi"` above becomes
`POST /api/hooks/wompi`.

## Wiring

`internal/adapters/http/router.go` blank-imports
`internal/core/hooks/providers` (so every file's `init()` runs and calls
`RegisterRoute`) and calls `rest.MountHooks(r, repos)` once, on the **public**
route group. Hook routes therefore do not require a CMS JWT — most callers
(external payment providers) can't supply one. If a hook needs to restrict who
can call it, validate a provider-specific signature/secret inside the handler
itself; don't rely on `Authenticate` middleware for that.

Tenant identity for a hook request is resolved the same way as every other
public route: from the `Host` header via `TenantResolver`. If a webhook
provider can't route by domain (e.g. a single global webhook URL shared
across tenants), resolve the tenant yourself inside the handler — e.g. by
looking up an account/reference id carried in the webhook payload — instead
of trusting a client-supplied tenant id.

## `HookContext`

```go
type HookContext struct {
    TenantID uuid.UUID
    Repos    Repos // Item, Collection, User, Role, Permission, Tenant (ports only)
}
```

`Repos` is a bundle of `internal/core/ports/outbound` interfaces, not the
concrete postgres adapter — `internal/core/hooks` is a core package and must
not import `internal/adapters/...` (see the dependency rule in `CLAUDE.md`).
Use `hc.Repos.Item` to read/write tenant collection rows, `hc.Repos.Tenant` to
look up tenant settings, etc.

## Constraints

- `RegisterRoute` must be called before `rest.MountHooks` runs (i.e. from an
  `init()` in a blank-imported package) — routes registered afterwards are
  never mounted.
- Route paths are static strings matched by chi; they support the same
  pattern syntax as any other chi route (e.g. `/wompi/{account}`).
- Handlers are plain `http.HandlerFunc`-shaped (`func(w, r, hc)`) — use
  `encoding/json` directly for request/response bodies; there's no automatic
  envelope like `writeJSON`/`writeError` in `internal/adapters/http/rest`
  (those are adapter-layer helpers hooks doesn't have access to).
