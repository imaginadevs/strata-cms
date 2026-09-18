---
name: create-migration
description: Scaffold a new goose SQL migration for the global (public) schema following this project's conventions. Use when adding or changing global tables such as public.tenants.
disable-model-invocation: true
---

# create-migration

Create a new **goose** migration for the **global** schema in
`internal/infrastructure/database/migrations/`.

## Critical rules

- **Migrations are immutable once applied.** NEVER edit an existing numbered file.
  Every change is a new file with the next sequential number.
- This directory is for **global** schema only (e.g. `public.tenants`). **Per-tenant
  collection tables are created at runtime via `ddl.go` — do NOT put tenant table DDL
  here.** If the request is about a collection/field/index for a tenant, stop and say
  so; it does not belong in a migration.
- Files are embedded with `go:embed` and run on startup by `database.Migrate`
  (tracking table `cms_migrations`, dialect `postgres`).

## Steps

1. Find the highest existing number:
   `ls internal/infrastructure/database/migrations/` (current highest: `001_tenants.sql`).
2. Create `NNN_<snake_case_description>.sql` with the next zero-padded number.
3. Use the goose annotation format below. Always write a real, reversible `Down`.
4. Qualify tables explicitly (`public.<table>`). Use `IF NOT EXISTS` where sensible.
5. Verify it compiles/embeds: `go build ./...`. Do not attempt to run it against a DB
   unless the user asks.

## Template

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS public.<table> (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- columns...
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS public.<table>;
-- +goose StatementEnd
```

Use one `StatementBegin/StatementEnd` block per statement that contains semicolons
(functions, `DO` blocks). Report the created path when done.
