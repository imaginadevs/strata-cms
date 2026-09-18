# Implementation Status

> Last updated: 2026-05-26 — Collection relations (DDL + REST + UI); field drag-and-drop reorder; item CRUD in Content page; session inet scan fix; system-collection field add  
> Legend: ✅ Done · 🔶 Partial / stub · ❌ Missing

---

## Backend

### Infrastructure
| File | Status | Notes |
|------|--------|-------|
| `infrastructure/config/config.go` | ✅ | All env vars loaded; AppConfig, DB, Auth, Kafka, S3, SuperAdmin |
| `infrastructure/database/postgres.go` | ✅ | pgxpool connect + Goose migrate runner |
| `infrastructure/database/migrations/001_initial_schema.sql` | ✅ | All system tables: tenants, users, roles, permissions, collections, fields, relations, audit_logs, languages |
| `infrastructure/database/migrations/002_super_admin_and_storage.sql` | ✅ | `is_super_admin` flag on users, `files` table |
| `infrastructure/database/migrations/003_tenant_storage_quota.sql` | ✅ | `storage_used_bytes`, PL/pgSQL `storage_claim` / `storage_release`, `storage_drift` view |
| `infrastructure/logger/logger.go` | ✅ | Zap with environment-based log level |
| `infrastructure/server/server.go` | ✅ | HTTP server with graceful shutdown |
| `infrastructure/bootstrap/bootstrap.go` | ✅ | First-run super admin seed from env; idempotent; provisions feature collections for all active tenants on startup |
| `infrastructure/static/embed.go` | ✅ | Embeds `frontend/dist` into binary |
| `cmd/server/main.go` | ✅ | Signal handling, health-check flag for Docker |

### Domain
| File | Status | Notes |
|------|--------|-------|
| `domain/base.go` | ✅ | `Base` struct: id, tenant_id, timestamps, actor IDs |
| `domain/tenant/entity.go` | ✅ | Tenant with `StorageAvailable()`, `StorageUsedPercent()`; `Settings.Features { Website, Blog }` |
| `domain/user/entity.go` | ✅ | User, Role, Permission, Action, Status, AuthProvider |
| `domain/collection/entity.go` | ✅ | Collection (base/auth), Field (25+ types), Relation, Validation, Translation; `AuthSystemFields()` |
| `domain/audit/entity.go` | ✅ | AuditLog with old/new data; `DiffFields()` |

### Ports / Interfaces
| File | Status | Notes |
|------|--------|-------|
| `ports/outbound/repositories.go` | ✅ | All 10 repository interfaces defined (Tenant, User, Role, Permission, Collection + DDL, Field, Relation, Item, Audit, Session) |
| `ports/outbound/storage.go` | ✅ | StorageAdapter, StorageFactory, `ErrQuotaExceeded` |

### Events
| File | Status | Notes |
|------|--------|-------|
| `core/events/bus.go` | ✅ | Bus interface + InMemoryBus (thread-safe, sync dispatch) |
| `core/events/events.go` | ✅ | 25+ EventType constants across data, schema, auth, tenant lifecycle |
| `adapters/events/kafka.go` | ✅ | KafkaBus via Watermill; topic = `{prefix}.{event_type}`; in-process hooks still run sync |

### HTTP Adapters
| File | Status | Notes |
|------|--------|-------|
| `adapters/http/router.go` | ✅ | Chi router; full middleware stack; `Authenticate` before `TenantResolver` in protected group so JWT-skip fires; route groups (public / JWT / SSE / SPA) |
| `adapters/http/middleware/auth.go` | ✅ | JWT parse; `Claims`; `Authenticate` + `AuthenticateOptional` |
| `adapters/http/middleware/tenant.go` | ✅ | Domain-only resolution (no X-Tenant-ID header — security decision); injects full `*Tenant` via `TenantFromCtx`; localhost → seed default tenant fallback; 404 on unknown domain, 403 on disabled tenant |
| `adapters/http/middleware/superadmin.go` | ✅ | IP/CIDR whitelist guard; reads from env only |
| `adapters/http/middleware/logger.go` | ✅ | Zap request logging |
| `adapters/http/rest/health.go` | ✅ | `GET /api/health` with DB ping |
| `adapters/http/rest/sse.go` | ✅ | SSE endpoint; `?token=` JWT auth; 25 s keepalive; per-tenant filter |
| `adapters/http/rest/auth.go` — login | ✅ | Tenant from ctx → user by email → bcrypt → session create → JWT |
| `adapters/http/rest/auth.go` — logout | ✅ | Delete session by refresh token (body or cookie) |
| `adapters/http/rest/auth.go` — refresh | ✅ | Validate session → re-issue JWT; checks expiry |
| `adapters/http/rest/auth.go` — me | ✅ | Fetches full user + roles from DB |
| `adapters/http/rest/auth.go` — SSO | ❌ | Logto OIDC stubs (501); implemented separately |
| `adapters/http/rest/tenants.go` | ✅ | `GET /tenants/current` (public); full CRUD (super admin / own tenant) |
| `adapters/http/rest/users.go` | ✅ | CRUD + set-password + assign/revoke role |
| `adapters/http/rest/roles.go` | ✅ | CRUD + list/set permissions per role |
| `adapters/http/rest/collections.go` | ✅ | CRUD + DDL (CreateTable/DropTable with compensating tx); reserved-name guard (422) |
| `adapters/http/rest/collections.go` — fields | ✅ | CRUD + AddColumn/DropColumn + `PUT /fields/reorder` (batch sort-index update) |
| `adapters/http/rest/relations.go` | ✅ | `GET/POST /collections/{c}/relations` + `DELETE /{id}`; dispatches DDL for M2O/1-1 (FK on current coll), 1-M (FK on related coll), M2M (junction table); `ensureFieldMeta` creates `type=relation` field row |
| `adapters/http/rest/items.go` | ✅ | Dynamic CRUD on per-tenant collection tables |
| `adapters/http/rest/audit.go` | ✅ | Paginated list with collection/user/operation filters |
| `adapters/http/rest/settings.go` | ✅ | GET + PATCH tenant settings/branding; triggers `EnsureFeatureCollections` on feature toggle |
| `core/features/features.go` | ✅ | `ReservedNames` map, `FeatureID`, collection blueprints (Website: pages; Blog: posts_categories + posts) |
| `core/features/ensure.go` | ✅ | `EnsureFeatureCollections` — idempotent provisioning; upgrades missing fields |
| `adapters/http/rest/graphql.go` | ✅ | Dynamic GraphQL: runtime schema per tenant, schema cache with event invalidation, GraphiQL playground, full CRUD mutations |

### Storage
| File | Status | Notes |
|------|--------|-------|
| `adapters/storage/s3.go` | ✅ | AWS SDK v2; AWS / R2 / MinIO support; presigned URLs; factory with bucket cache |

### Persistence (Postgres) — ✅ COMPLETE
| File | Status | Notes |
|------|--------|-------|
| `adapters/persistence/postgres/postgres.go` | ✅ | `tenantSchema()` helper; `Repos` bundle + `NewRepos(db)` constructor |
| `adapters/persistence/postgres/ddl.go` | ✅ | DDL adapter; `safeIdent()` injection guard; `pgType()` mapping; fully-qualified `cms_{hex8}.table` names; `AddForeignKey` (nullable UUID FK + optional UNIQUE); `CreateJunctionTable` (composite PK junction) |
| `adapters/persistence/postgres/tenant_repository.go` | ✅ | Full CRUD; `Create` opens TX + `CREATE SCHEMA IF NOT EXISTS cms_{hex8}` atomically |
| `adapters/persistence/postgres/user_repository.go` | ✅ | CRUD + `SetPassword` + `RecordLogin`; eager-loads roles |
| `adapters/persistence/postgres/session_repository.go` | ✅ | Create / GetByToken / Delete / DeleteAllForUser; `ip_address::text` cast fixes pgx binary inet scan |
| `adapters/persistence/postgres/role_repository.go` | ✅ | CRUD + `AssignToUser` / `RevokeFromUser` |
| `adapters/persistence/postgres/permission_repository.go` | ✅ | Upsert / Delete / `GetForRole` / `GetForUser` (joins user_roles) |
| `adapters/persistence/postgres/collection_repository.go` | ✅ | Metadata CRUD + DDL pass-through; eager-loads fields |
| `adapters/persistence/postgres/field_repository.go` | ✅ | CRUD; shared `scanFieldRow` with collection_repository |
| `adapters/persistence/postgres/relation_repository.go` | ✅ | Create / List (by either side) / Delete |
| `adapters/persistence/postgres/item_repository.go` | ✅ | Dynamic CRUD on `cms_{hex8}.{collection}` tables; `pgxRowsToMap` for schema-less scan |
| `adapters/persistence/postgres/audit_repository.go` | ✅ | Append-only Create; paginated List with collection/user/operation filters |

---

## Frontend

### Foundation
| File | Status | Notes |
|------|--------|-------|
| `vite.config.ts` | ✅ | /api proxy, SSE passthrough, manual chunks |
| `tailwind.config.js` | ✅ | Brand palette, `app-bg` periwinkle, `shadow-card`, animations |
| `src/styles/globals.css` | ✅ | `glass-card`, `btn-primary`, `btn-ghost`, `.input`, badges |
| `src/types/index.ts` | ✅ | All TypeScript interfaces mirroring backend entities |
| `src/lib/api.ts` | ✅ | Axios; Bearer inject; 401 refresh queue; redirect to /login |
| `src/lib/sse.ts` | ✅ | SSEClient; exponential backoff (1 s → 30 s); wildcard listeners |
| `src/lib/utils.ts` | ✅ | `cn()`, `initials()`, `formatDate()`, `truncate()`, `deepClone()` |
| `src/store/auth.store.ts` | ✅ | Zustand + Immer + persist; `isTokenExpired()` with 1-min buffer |
| `src/store/tenant.store.ts` | ✅ | Zustand + Immer + persist; active language |
| `src/routes/index.tsx` | ✅ | TanStack Router; auth guard; lazy-loaded pages |

### Layout & Components
| File | Status | Notes |
|------|--------|-------|
| `components/layout/AppShell.tsx` | ✅ | Lavender bg; Header + main as white `rounded-2xl` cards; SSE mount |
| `components/layout/Header.tsx` | ✅ | Search bar; notification bell; user dropdown (Radix) |
| `components/layout/Sidebar.tsx` | ✅ | Icon-box nav; centered avatar profile; `group-data-[status=active]` active state |
| `components/ui/Button.tsx` | ✅ | CVA variants (primary, ghost, destructive) |
| `components/ui/Card.tsx` | ✅ | Card layout primitive |
| `components/ui/PageSpinner.tsx` | ✅ | Centered loading spinner |

### Pages
| File | Status | Notes |
|------|--------|-------|
| `pages/Login.tsx` | ✅ | RHF + Zod; Framer Motion; calls `POST /auth/login` |
| `pages/Dashboard.tsx` | 🔶 | Stat cards wired to API; data depends on working backend |
| `pages/Collections.tsx` | ✅ | List/detail views with Framer Motion transitions; full CRUD (collection + field + relation); field drag-and-drop reorder via @dnd-kit (optimistic, reverts on error); system collections allow Add Field but protect existing system fields; Relations section with M2O/1-1/1-M/M2M panel |
| `pages/Content.tsx` | ✅ | Dynamic item browser per collection; `FieldInput` maps all 25+ field types to native inputs; `ItemPanel` slide-in (create/edit, required validation, server errors); `DeleteConfirm` modal; boolean badge display |
| `pages/Users.tsx` | 🔶 | List skeleton; **no CRUD** |
| `pages/Settings.tsx` | 🔶 | Feature toggles (Website/Blog) wired and saved; S3 + SSO form placeholders not yet saved |
| `pages/Audit.tsx` | 🔶 | Table skeleton; **no data** |
| `pages/NotFound.tsx` | ✅ | 404 page |

---

## Documentation
| File | Status |
|------|--------|
| `docs/Architecture.md` | ✅ |
| `docs/implementations.md` | ✅ |
| `docs/implementation/01-project-setup.md` | ✅ |
| `docs/implementation/02-backend-foundation.md` | ✅ |
| `docs/implementation/03-database-schema.md` | ✅ |
| `docs/implementation/04-auth-system.md` | ✅ |
| `docs/implementation/05-multi-tenancy.md` | ✅ |
| `docs/implementation/06-collection-engine.md` | ✅ |
| `docs/implementation/07-permissions.md` | ✅ |
| `docs/implementation/08-api-layer.md` | ✅ |
| `docs/implementation/09-events-hooks.md` | ✅ |
| `docs/implementation/10-i18n.md` | ✅ |
| `docs/implementation/11-file-storage.md` | ✅ |
| `docs/implementation/12-deployment.md` | ✅ |

---

## Summary

| Layer | Progress |
|-------|----------|
| Infrastructure (config, DB, migrations, server, bootstrap) | ✅ 100% |
| Domain entities | ✅ 100% |
| Port interfaces | ✅ 100% |
| Event system (InMemory + Kafka) | ✅ 100% |
| HTTP router + middleware | ✅ 100% |
| Storage adapter (S3) | ✅ 100% |
| Frontend foundation + layout | ✅ 100% |
| **Postgres repositories** | **✅ 100%** |
| HTTP REST handlers | ✅ 98% (SSO remaining) |
| Frontend pages (data wired) | 🔶 70% |

### Implementation order
1. ~~`adapters/persistence/postgres/` — DDL adapter + all repositories~~ ✅ done
2. `rest/auth.go` — wire login / logout / refresh / me to real repositories
3. `rest/users.go` — full CRUD
4. `rest/collections.go` + `rest/fields.go` — schema management with DDL
5. `rest/items.go` — dynamic item CRUD
6. `rest/audit.go`, `rest/roles.go`, `rest/settings.go`
7. SSO (Logto OIDC flow)
8. GraphQL schema + resolvers
9. Frontend pages: wire Collections, Users, Settings, Audit to live API
