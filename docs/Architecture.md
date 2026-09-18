# Architecture Reference

> **Keep this file updated.** Pass it to Claude at the start of every session so it has full context of the system without re-reading the codebase.

## Overview

A headless CMS built in Go (backend) + React (frontend). PostgreSQL is the only database. Events flow through an internal bus (dev) or Kafka (prod). REST and GraphQL are both supported. Every entity is multi-tenant, identified by `tenant_id`.

---

## Technology decisions

| Concern | Choice | Reason |
|---------|--------|--------|
| HTTP router | `go-chi/chi` | Lightweight, idiomatic Go, composable middleware |
| DB driver | `jackc/pgx v5` | Fastest PostgreSQL driver; native pgx pool |
| Migrations | `pressly/goose` | Embedded SQL files, clean Up/Down |
| GraphQL | `99designs/gqlgen` | Code-first, strongly typed, no reflection |
| Event bus | `InMemoryBus` (dev) / `watermill-kafka` (prod) | Same `Bus` interface, swappable without callers changing |
| Realtime | SSE (not WebSocket) | Server-push only; HTTP-compatible; auto-reconnect; HTTP/2 friendly |
| Logger | `uber-go/zap` | Structured, zero-alloc |
| Config | `spf13/viper` | .env + environment variables |
| Auth tokens | `golang-jwt/jwt v5` | Standard HS256 JWTs |
| SSO | Logto (OIDC) | Optional; wired via env flags |
| File storage | AWS SDK v2 (S3-compatible) | Per-tenant; supports AWS, R2, MinIO |
| Frontend bundler | Vite 5 | Fastest HMR; optimal chunk splitting |
| Frontend routing | TanStack Router v1 | Type-safe, intent-preloading |
| Frontend state | Zustand v4 + Immer | Minimal, composable |
| Frontend data | TanStack Query v5 | Server state, caching, background refresh |
| UI components | Radix UI + Tailwind | Accessible headless primitives; glass/crystal aesthetic |

---

## Directory structure

```
backend/
  cmd/
    server/main.go              Entry point — wires all dependencies
  internal/
    core/                       Pure domain (no framework imports)
      domain/
        base.go                 BaseEntity: id, tenant_id, created_at, updated_at, created_by, updated_by
        tenant/entity.go        Tenant: name, slug, default_language, settings (JSONB)
        user/entity.go          User + Role + Permission types
        collection/entity.go    Collection (model def) + Field + Relation + CollectionType
        audit/entity.go         AuditLog: who changed what, old/new data
      ports/
        outbound/repositories.go  All DB interface contracts
        outbound/storage.go       StorageAdapter, StorageFactory, FileRepository
      events/
        bus.go                  Bus interface + InMemoryBus implementation
        events.go               EventType constants + Event struct
    adapters/
      http/
        router.go               Chi router; mounts all route groups
        middleware/
          auth.go               JWT parse → context; Authenticate / AuthenticateOptional
          tenant.go             Tenant resolution (header or domain)
          superadmin.go         IP whitelist enforcement for super admins
          logger.go             Zap request logger
        rest/
          health.go             GET /api/health
          auth.go               POST /api/auth/login|logout|refresh · GET /api/auth/me
                                SSO redirect + callback (when ENABLE_SSO=true)
          (stubs)               Users, Roles, Collections, Fields, Items, Audit, WS, GraphQL
      events/
        kafka.go                KafkaBus — Watermill + Kafka; implements core/events.Bus
      storage/
        s3.go                   S3Adapter — AWS SDK v2; implements outbound.StorageAdapter
                                Factory caches adapters per tenant bucket
    infrastructure/
      config/config.go          AppConfig, ServerConfig, DatabaseConfig, AuthConfig,
                                LogtoConfig, KafkaConfig, SuperAdminConfig, FeatureFlags
      database/
        postgres.go             pgxpool.Connect + Goose migrations runner
        migrations/
          001_initial_schema.sql  tenants, users, roles, permissions, sessions,
                                  collections, fields, relations, audit_logs, languages
          002_super_admin_and_storage.sql  is_super_admin flag, files table
      logger/logger.go          Zap SugaredLogger factory
      server/server.go          http.Server lifecycle (Start / Shutdown)
      static/embed.go           embed.FS wrapping frontend/dist (production)

frontend/
  src/
    routes/index.tsx            TanStack Router tree (public + protected with AppShell guard)
    store/
      auth.store.ts             isAuthenticated, accessToken, user — persisted to localStorage
      tenant.store.ts           tenant, activeLanguage — persisted to localStorage
    lib/
      api.ts                    Axios client; /api base; auto-refresh on 401
      graphql.ts                graphql-request client factory (reads token from auth store)
      sse.ts                    SSEClient: connect to /api/events, fan-out to typed listeners
      utils.ts                  cn(), formatDate(), truncate(), initials(), deepClone()
    types/index.ts              All TypeScript interfaces mirroring backend domain
    components/
      layout/
        AppShell.tsx            Root layout: mounts SSE, renders Sidebar + Header + main
        Sidebar.tsx             Glass nav with tenant branding
        Header.tsx              Search, notifications, user menu (logout)
      ui/
        Button.tsx              CVA variants: primary, secondary, ghost, danger, outline
        Card.tsx                Glass card + CardHeader + CardTitle
        PageSpinner.tsx         Full-screen spinner for Suspense boundaries
    pages/
      Login.tsx                 Email/password form + SSO link
      Dashboard.tsx             Stats grid + recent collections + recent audit
      Collections.tsx           Collection list with type filter (base/auth)
      Users.tsx                 User table with status/provider badges
      Audit.tsx                 Audit log table
      Settings.tsx              General, S3 storage, SSO configuration
      NotFound.tsx              404
```

---

## Domain model

### BaseEntity (embedded in every entity)

```
id          UUID  PRIMARY KEY  DEFAULT uuid_generate_v4()
tenant_id   UUID  NOT NULL REFERENCES tenants(id)
created_at  TIMESTAMPTZ
updated_at  TIMESTAMPTZ  (auto-updated via trigger)
created_by  UUID  REFERENCES users(id)
updated_by  UUID  REFERENCES users(id)
```

### Collection types

| Type | Behaviour |
|------|-----------|
| `base` | Standard collection; no special fields |
| `auth` | Auto-injects system fields: `username`, `password_hash`, `status`, `last_login`. Integrates with auth engine |

### System collections

Collections with `is_system = true` are created by migrations and cannot be modified or deleted via the API. System roles: Administrator, Editor, Viewer.

---

## Multi-tenancy

Every query **must** include `WHERE tenant_id = $1`. The tenant is resolved in this order:

1. JWT claim `tid` (most trusted — set at login time)
2. `X-Tenant-ID` header (slug or UUID)
3. Host header match against `tenants.domain`

Super admins (JWT role `super_admin`) bypass tenant isolation and can query any tenant. Their requests are additionally gated by `SUPER_ADMIN_ALLOWED_IPS` from `.env` (never the database).

---

## Auth flow

### username/password
```
POST /api/auth/login  →  verify bcrypt hash  →  issue JWT (24h) + refresh token (7d)
                      →  publish events.EventUserLogin
```

### SSO (Logto / OIDC)
```
GET /api/auth/sso         →  redirect to Logto authorization URL
GET /api/auth/sso/callback →  exchange code for tokens → upsert user → issue JWT
```

### JWT payload
```json
{
  "sub":   "<user UUID>",
  "tid":   "<tenant UUID>",
  "email": "user@example.com",
  "roles": ["Administrator"],
  "exp":   1234567890
}
```

---

## Event system

### Bus interface

```go
type Bus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType EventType, handler Handler) Subscription
    Unsubscribe(sub Subscription)
}
```

All model operations must call `bus.Publish()` before returning. Handlers registered with `Subscribe()` run synchronously in the same goroutine — returning an error aborts the operation (guard hook pattern).

### Built-in event types

```
items.before.create / items.after.create
items.before.read   / items.after.read
items.before.update / items.after.update
items.before.delete / items.after.delete

schema.collection.created / .updated / .deleted
schema.field.created      / .updated / .deleted

auth.login / auth.logout / auth.user.created / auth.password.changed
tenant.created / tenant.updated
```

### Kafka

`KafkaBus` (adapters/events/kafka.go) publishes each event to a topic named `<prefix>.<event_type>` (e.g. `cms.items.after.create`). Swap by setting `ENABLE_KAFKA=true` + `KAFKA_BROKERS` in `.env`. The `Bus` interface is unchanged.

---

## Realtime (SSE)

```
GET /api/events?token=<jwt>
```

- Token in query param (EventSource doesn't support custom headers)
- Server sends `data: <json>\n\n` per event
- Frontend `sseClient` (src/lib/sse.ts) reconnects with exponential backoff (1s → 30s max)
- Used for: live data updates, notifications, schema change broadcast

---

## File storage (S3)

`StorageFactory` (adapters/storage/s3.go) creates an `S3Adapter` per tenant using their stored credentials. S3 secrets are encrypted at rest in `tenants.settings.storage_config`. File metadata is persisted in the `files` table; binary data lives in S3 only.

Supported providers: AWS S3, Cloudflare R2, MinIO (set `FORCE_PATH_STYLE=true`), any S3-compatible API.

---

## API conventions

### REST

```
Base path: /api
Auth:      Authorization: Bearer <jwt>
Tenant:    X-Tenant-ID: <slug|uuid>  (or resolved from JWT)

GET    /api/items/{collection}?page=1&limit=25&sort=-created_at&filter[status]=active
POST   /api/items/{collection}
GET    /api/items/{collection}/{id}
PATCH  /api/items/{collection}/{id}
DELETE /api/items/{collection}/{id}

GET    /api/collections
POST   /api/collections
PATCH  /api/collections/{name}
DELETE /api/collections/{name}

GET    /api/users
POST   /api/auth/login
GET    /api/health
GET    /api/events         (SSE)
POST   /api/graphql
```

### Response envelope

```json
{
  "data": [...],
  "total": 120,
  "page": 1,
  "limit": 25
}
```

Errors:
```json
{ "error": "not found" }
{ "error": "validation failed", "details": { "email": ["invalid format"] } }
```

### GraphQL

Endpoint: `POST /api/graphql`  
Playground: `GET /api/graphql` (development only)  
Schema is dynamically generated per tenant from collection definitions.

---

## Permissions model

```
Role  1──* Permission
User *──* Role

Permission {
  collection: "*" | "posts" | ...
  action:     create | read | update | delete
  fields:     ["*"] | ["title", "body"]
  filter:     { "status": { "_eq": "published" } }  // row-level
}
```

System roles (Administrator, Editor, Viewer) cannot be deleted. Super admins bypass all permission checks.

---

## i18n

- Tenant has `default_language`
- `languages` table tracks enabled languages per tenant
- Fields with `translatable: true` store values in a `_translations` shadow table (one row per language)
- API returns the `lang` query param language, falling back to default
- Frontend language selector creates translations for the same record in a different language — not a duplicate record

---

## Super admin

- Stored in `users.is_super_admin = true`
- JWT includes role `super_admin`
- `SuperAdminGate` middleware (adapters/http/middleware/superadmin.go) checks `SUPER_ADMIN_ALLOWED_IPS` from config
- Filtered out of all regular `GET /api/users` responses
- Can pass `X-Tenant-ID` to operate on any tenant

---

## Docker build

Three-stage Dockerfile:
1. **frontend-builder** (node:20-alpine) — `npm ci && npm run build`
2. **backend-builder** (golang:1.22-alpine) — copies `frontend/dist` into `internal/infrastructure/static/dist`, then `go build` with `//go:embed dist`
3. **runtime** (gcr.io/distroless/static-debian12) — single binary, no shell, non-root

Only port 8080 is exposed. The Go binary serves both `/api/*` and the static SPA.

---

## Adding a new feature — checklist

1. Define domain entity in `internal/core/domain/`
2. Add repository interface to `internal/core/ports/outbound/repositories.go`
3. Write SQL migration in `internal/infrastructure/database/migrations/`
4. Implement repository in `internal/adapters/persistence/postgres/`
5. Implement HTTP handler in `internal/adapters/http/rest/`
6. Mount handler in `internal/adapters/http/router.go`
7. Publish events via `bus.Publish()` on create/update/delete
8. Add TypeScript types to `frontend/src/types/index.ts`
9. Add API hook in `frontend/src/lib/api.ts`
10. Add TanStack Query hook in the relevant page/component
