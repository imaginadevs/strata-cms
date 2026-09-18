# CLAUDE.md

Guidance for Claude Code when working in this repository.

## What this is

A **multitenant, headless CMS** (Directus-like) written in **Go 1.25**. It exposes
dynamic content collections over **REST + GraphQL + SSE**, with schema, fields,
relations, indexes, roles/permissions and settings all defined at runtime per
tenant. Performance and tenant isolation are the two overriding concerns — treat
both as correctness requirements, not nice-to-haves.

Module: `github.com/imaginacolombia/cms`

## Architecture (hexagonal / ports & adapters)

```
cmd/server/main.go            Entry point: config → db → migrate → bootstrap → server
internal/core/                Business core — no framework/driver imports
  domain/                     Entities: audit, collection, tenant, user
  ports/                      Interfaces the core depends on
  features/, events/          Use-case wiring, domain events
  autogen/                    Runtime schema/type generation (builtins, registry)
internal/adapters/            Implementations of ports
  http/                       chi router, middleware (auth, tenant, superadmin), rest handlers
  persistence/postgres/       pgx repositories + ddl.go (runtime DDL)
  events/                     Kafka (Watermill) event publishing
  storage/                    S3-compatible object storage
internal/infrastructure/      Process wiring: config, database, logger, server, bootstrap, static
```

**Dependency rule:** `core` must not import `adapters` or `infrastructure`, and must
not import drivers (pgx, chi, aws, kafka). Dependencies point inward. When adding a
capability the core needs, define a port in `internal/core/ports` and implement it in
an adapter.

## Multitenancy — the #1 invariant

Tenants are isolated by **schema-per-tenant** in a single Postgres database:

- Each tenant gets a Postgres schema; see `schemaOf(tenantID)` in
  `internal/adapters/persistence/postgres/ddl.go`.
- **Every** table reference in queries and DDL must be **fully qualified**
  (`schema.table`). There is no `search_path` reliance — this is deliberate so it
  works with **PgBouncer in transaction-pool mode**. Never introduce an unqualified
  table name in a per-tenant query.
- Runtime DDL builds SQL by string interpolation. **All** identifiers must be
  validated with `safeIdentRe` (`^[a-zA-Z_][a-zA-Z0-9_]*$`) before they touch a SQL
  string. Never interpolate a caller-supplied identifier that hasn't passed
  validation — this is the primary SQL-injection surface.
- Tenant identity is resolved from the **JWT** (authoritative) or the **Host header**
  (domain match), never from a client-supplied `X-Tenant-ID` header. See
  `internal/adapters/http/middleware/tenant.go`.
- `public.tenants` is the only global table; it is migrated by goose (see below).

When reviewing or writing DB code, always ask: *is this scoped to the right tenant
schema, and is every dynamic identifier validated?*

## Migrations

- Global migrations (the `public.tenants` table etc.) live in
  `internal/infrastructure/database/migrations/NNN_name.sql`, run by **goose** on
  startup (`database.Migrate`, tracking table `cms_migrations`). Files use
  `-- +goose Up` / `-- +goose Down` and are embedded via `go:embed`.
- **Migrations are immutable once applied.** Never edit an existing numbered
  migration — add a new one with the next number (current: `001_tenants.sql`).
- Per-tenant collection tables are created at **runtime** via `ddl.go`, *not* through
  goose.

## Build / run / check

```bash
go build ./...          # compile everything
go vet ./...            # static checks (main safety net — see testing note)
gofmt -l .              # list unformatted files; gofmt -w . to fix
go run ./cmd/server     # run locally (reads .env or env vars via viper)
go run ./cmd/server --health   # health probe used by Docker HEALTHCHECK
```

Config is loaded by **viper** from `.env` (or real env vars) with sensible defaults
(`internal/infrastructure/config/config.go`). Server defaults to `:8080`. On startup
the app connects to Postgres, runs goose migrations, then `bootstrap.Run` (seeds the
super-admin from env on first start only).

## Testing

There is currently **no test suite**. The seams exist (`stdlibDB` wraps the pool for
packages needing `*sql.DB`). Until coverage exists, `go build ./...` and `go vet ./...`
are the feedback loop — run them after changes. New tests are very welcome,
especially around `persistence/postgres` (assert tenant-schema scoping) and the HTTP
middleware.

## Conventions

- Logging: **zap** (`internal/infrastructure/logger`). Don't use the stdlib `log`.
- Config: never read env vars directly outside `config.go`; add a field to `Config`.
- Errors: wrap with `fmt.Errorf("...: %w", err)`; the codebase already does this.
- Secrets (`JWT_SECRET`, `SUPER_ADMIN_PASSWORD`, S3 keys, DB password) come from env
  only — never hardcode, log, or commit them.
- Feature flags gate REST/GraphQL/SSO/Kafka (`FeatureFlags`); respect them.
