# Technical Debt

Intentional shortcuts that **work today** but should be revisited. Distinct from
[BACKLOG.md](BACKLOG.md) (new features to build) — this is deferred
infrastructure/consistency work.

Format per entry: what · why deferred · impact if left · resolution · revisit-when.

---

## UUIDv7 at the database level

**Status:** Deferred · **Introduced:** 2026-07-23

**What.** Entity primary keys are UUIDv7, but only because the Go app mints them
(`internal/core/domain/ids` → `ids.New()`). The Postgres column DEFAULTs still
generate **v4**:
- `migrations/001_tenants.sql` and `database/tenant_migrations.go` → `gen_random_uuid()`
- `adapters/persistence/postgres/ddl.go` (runtime collection tables) → `uuid_generate_v4()`

(Also note these two v4 defaults are inconsistent — core `pgcrypto` vs `uuid-ossp`.)

**Why deferred.** The database is **PostgreSQL 16** (`docker-compose.yml`:
`postgres:16-alpine`), which has **no native `uuidv7()`** — that arrived in
PostgreSQL 18. The app supplies `id` on effectively every insert, so rows are v7 in
practice; the column DEFAULT is only used if an insert omits `id`. Continuing with
Go-side generation for now.

**Impact if left.** Minor. A row inserted **without** an app-supplied `id` (a manual
SQL insert, or a future code path that forgets to set `id`) gets a v4 id — losing
index locality for that row and mixing versions in the column. No correctness issue;
v4 and v7 coexist fine in a `uuid` column.

**Resolution (when revisited).**
1. Add a `uuid_generate_v7()` plpgsql function via a **new** migration (works on
   PG 13+), switch the `ddl.go` runtime default + future tenant tables to it, and
   standardise on one function name. Existing tables keep their v4 default
   (migrations are immutable) — harmless, no data migration.
2. Or upgrade to **PostgreSQL 18** and use native `uuidv7()`.

**Check current DB support:** `SELECT uuidv7();` (errors on < 18);
`SELECT version();`.

**Revisit when:** upgrading Postgres, or if manual/SQL inserts (outside the app)
become common.

---

## Related deferrals (introduced with the relational I/O work)

Low-priority, documented in the relevant design docs; listed here so they aren't lost.

- **Untyped (non-indexed) item filtering.** `item_repository.go` `List` compares
  filter values on `::text`, so equality filters don't use an index yet. Fine for the
  opt-in filtering use case. Revisit with typed filtering + a
  `pages_blocks(page_id, sort)` index. (See `relational-items.md` → Gaps.)
- **Metadata-only referential integrity.** Relations are metadata rows, not physical
  FKs/cascade (matching `posts → posts_categories`). Deleting a parent does not
  DB-cascade to owned children outside a `SaveGraph` call. (See
  `design-relational-io.md`.)
- **No DB integration tests** for tenant-schema scoping and the nested-write paths.
  The `gen-repo-test` skill exists to add them (assert scoping, owned-delete, and the
  within-tenant reparent guard).
