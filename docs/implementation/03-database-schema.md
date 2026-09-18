# 03 — Database Schema

## Overview

The CMS has two categories of database tables: static system tables defined in embedded Goose SQL migrations, and dynamic collection tables created at runtime via DDL when a user creates a collection through the API. This guide covers both: running and writing migrations, and understanding how the collection engine materializes user-defined schemas as real PostgreSQL tables.

## Prerequisites

- Guide 01 completed (PostgreSQL running, `.env` configured).
- `make setup` has been run.

## Step-by-Step

### 1. How Goose migrations are embedded

The `database.Migrate()` function in `backend/internal/infrastructure/database/postgres.go` uses Go's `embed.FS` to bundle all `.sql` files into the binary:

```go
//go:embed migrations/*.sql
var migrations embed.FS

func Migrate(cfg config.DatabaseConfig) error {
    db, err := sql.Open("pgx", cfg.DSN)
    // ...
    goose.SetBaseFS(migrations)
    goose.SetDialect("postgres")
    goose.SetTableName("cms_migrations")   // custom tracking table name

    return goose.Up(db, "migrations")
}
```

Migration state is tracked in `cms_migrations` (not the default `goose_db_version`). This means you can verify which migrations have run:

```bash
psql $DATABASE_URL -c "SELECT version, applied_at FROM cms_migrations ORDER BY version;"
```

### 2. Migration file naming convention

Goose uses the numeric prefix to determine migration order:

```
001_initial_schema.sql
002_super_admin_and_storage.sql
003_add_pages.sql        ← next one you create
```

Always use three-digit zero-padded prefixes. Do not rename existing migration files — Goose tracks them by version number embedded in the filename.

### 3. Create a new migration

```bash
make migrate-create name=add_pages
# Equivalent to: cd backend && go run ./cmd/migrate/main.go create add_pages sql
```

This creates a file like:

```
backend/internal/infrastructure/database/migrations/003_add_pages.sql
```

### 4. Migration file format

Every migration file must follow the Goose comment format:

```sql
-- +goose Up
-- +goose StatementBegin

-- Your SQL here — DDL or DML
CREATE TABLE pages (
    id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title      VARCHAR(500) NOT NULL,
    slug       VARCHAR(500) NOT NULL,
    status     VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (tenant_id, slug)
);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

DROP TABLE IF EXISTS pages;

-- +goose StatementEnd
```

Rules:
- `-- +goose StatementBegin` / `-- +goose StatementEnd` are required when the SQL contains semicolons inside function bodies (PL/pgSQL). They are optional for simple DDL but recommended for consistency.
- The `Down` section must undo everything in `Up`. Test it: `make migrate-down && make migrate-up`.
- Never use `DROP TABLE` without `IF EXISTS` in the Down section.

### 5. Apply and roll back migrations

```bash
# Apply all pending migrations
make migrate-up

# Roll back one migration
make migrate-down

# Check migration status (standalone runner)
cd backend && go run ./cmd/migrate/main.go status
```

### 6. System table overview

After `make migrate-up`, the following tables exist in PostgreSQL:

| Table | Purpose |
|-------|---------|
| `tenants` | Top-level isolation units; every row elsewhere has `tenant_id` |
| `languages` | Per-tenant active languages |
| `users` | Built-in admin users (not dynamic auth collections) |
| `roles` | Named permission groups per tenant |
| `user_roles` | M-N join table |
| `sessions` | Refresh token storage |
| `permissions` | Role → collection → action → fields/filter |
| `collections` | Model definitions (metadata) |
| `collection_translations` | Translated display names |
| `fields` | Column definitions per collection |
| `field_translations` | Translated field display names |
| `relations` | Inter-collection relationships |
| `audit_logs` | Immutable operation log |
| `files` | Uploaded file metadata |
| `cms_migrations` | Goose state table |

### 7. Dynamic collection tables — the runtime DDL pattern

When a user creates a collection via `POST /api/collections`, the backend:

1. Inserts a row in the `collections` table (metadata).
2. Inserts rows in the `fields` table for each declared field.
3. Calls `CollectionRepository.CreateTable()` which executes DDL against the live schema.

The `CollectionRepository` interface declares the DDL operations:

```go
// From internal/core/ports/outbound/repositories.go
type CollectionRepository interface {
    // ...metadata CRUD...
    CreateTable(ctx context.Context, tenantID uuid.UUID, c *collection.Collection) error
    DropTable(ctx context.Context, tenantID uuid.UUID, name string) error
    AddColumn(ctx context.Context, tenantID uuid.UUID, f *collection.Field) error
    AlterColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collection.Field) error
    DropColumn(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error
}
```

### 8. Implementing CreateTable in the PostgreSQL adapter

Create `backend/internal/adapters/postgres/collection_ddl.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "strings"

    "github.com/google/uuid"

    "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

// CreateTable generates and executes a CREATE TABLE statement for a user-defined
// collection. System fields (id, tenant_id, created_at, etc.) are always injected.
func (r *CollectionRepository) CreateTable(
    ctx context.Context,
    tenantID uuid.UUID,
    c *collection.Collection,
) error {
    cols := []string{
        "id         UUID        PRIMARY KEY DEFAULT uuid_generate_v4()",
        "tenant_id  UUID        NOT NULL",
        "created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
        "updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
        "created_by UUID",
        "updated_by UUID",
    }

    // Inject auth system fields before user-defined fields
    allFields := c.Fields
    if c.Type == collection.CollectionTypeAuth {
        systemFields := collection.AuthSystemFields(c.Name, tenantID)
        allFields = append(systemFields, allFields...)
    }

    for _, f := range allFields {
        col := buildColumnDDL(f)
        if col != "" {
            cols = append(cols, col)
        }
    }

    ddl := fmt.Sprintf(
        "CREATE TABLE IF NOT EXISTS %s (\n    %s\n)",
        pgIdentifier(c.Name),
        strings.Join(cols, ",\n    "),
    )

    if _, err := r.db.Exec(ctx, ddl); err != nil {
        return fmt.Errorf("CreateTable %s: %w", c.Name, err)
    }

    // Always create tenant isolation index
    _, err := r.db.Exec(ctx, fmt.Sprintf(
        "CREATE INDEX IF NOT EXISTS idx_%s_tenant ON %s (tenant_id)",
        c.Name, pgIdentifier(c.Name),
    ))
    return err
}

// AddColumn executes ALTER TABLE ... ADD COLUMN for a single new field.
func (r *CollectionRepository) AddColumn(
    ctx context.Context,
    _ uuid.UUID,
    f *collection.Field,
) error {
    col := buildColumnDDL(*f)
    if col == "" {
        return nil
    }
    ddl := fmt.Sprintf(
        "ALTER TABLE %s ADD COLUMN IF NOT EXISTS %s",
        pgIdentifier(f.CollectionName), col,
    )
    _, err := r.db.Exec(ctx, ddl)
    return err
}

// AlterColumn changes the column type. PostgreSQL requires explicit USING cast
// for type changes — callers should validate compatibility before calling this.
func (r *CollectionRepository) AlterColumn(
    ctx context.Context,
    _ uuid.UUID,
    collectionName string,
    f *collection.Field,
) error {
    ddl := fmt.Sprintf(
        "ALTER TABLE %s ALTER COLUMN %s TYPE %s USING %s::%s",
        pgIdentifier(collectionName),
        pgIdentifier(f.Name),
        f.DBType,
        pgIdentifier(f.Name),
        f.DBType,
    )
    _, err := r.db.Exec(ctx, ddl)
    return err
}

// DropColumn removes a column. Data is irrecoverably lost.
func (r *CollectionRepository) DropColumn(
    ctx context.Context,
    _ uuid.UUID,
    collectionName, fieldName string,
) error {
    ddl := fmt.Sprintf(
        "ALTER TABLE %s DROP COLUMN IF EXISTS %s",
        pgIdentifier(collectionName),
        pgIdentifier(fieldName),
    )
    _, err := r.db.Exec(ctx, ddl)
    return err
}

// DropTable removes the entire collection table. Called on collection delete.
func (r *CollectionRepository) DropTable(
    ctx context.Context,
    _ uuid.UUID,
    name string,
) error {
    _, err := r.db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", pgIdentifier(name)))
    return err
}

// ─── DDL helpers ──────────────────────────────────────────────────────────────

// buildColumnDDL converts a Field definition to a PostgreSQL column clause.
func buildColumnDDL(f collection.Field) string {
    if f.DBType == "" {
        return ""
    }

    parts := []string{pgIdentifier(f.Name), f.DBType}

    if f.Required {
        parts = append(parts, "NOT NULL")
    }

    if f.DefaultValue != nil {
        parts = append(parts, fmt.Sprintf("DEFAULT %v", f.DefaultValue))
    }

    if f.Unique {
        parts = append(parts, "UNIQUE")
    }

    return strings.Join(parts, " ")
}

// pgIdentifier safely quotes a PostgreSQL identifier to prevent SQL injection.
// Only alphanumeric + underscore identifiers are accepted; anything else panics
// at startup so misconfigurations surface early.
func pgIdentifier(name string) string {
    // Simple validation — collection/field names must be snake_case
    for _, ch := range name {
        if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
            panic(fmt.Sprintf("unsafe SQL identifier: %q", name))
        }
    }
    return `"` + name + `"`
}
```

### 9. FieldType to DBType mapping

When creating fields, the API accepts a logical `type` (e.g. `"string"`) and the adapter maps it to a PostgreSQL column type:

```go
// Example mapping helper (add to collection_ddl.go)
func FieldTypeToDBType(ft collection.FieldType) string {
    switch ft {
    case collection.FieldTypeString:
        return "varchar(255)"
    case collection.FieldTypeText, collection.FieldTypeRichText:
        return "text"
    case collection.FieldTypeInteger:
        return "integer"
    case collection.FieldTypeFloat:
        return "double precision"
    case collection.FieldTypeDecimal:
        return "numeric(18,4)"
    case collection.FieldTypeBoolean:
        return "boolean"
    case collection.FieldTypeDate:
        return "date"
    case collection.FieldTypeDateTime, collection.FieldTypeTimestamp:
        return "timestamptz"
    case collection.FieldTypeJSON:
        return "jsonb"
    case collection.FieldTypeUUID, collection.FieldTypeRelation:
        return "uuid"
    case collection.FieldTypeFile, collection.FieldTypeFiles:
        return "uuid"   // references files.id
    case collection.FieldTypeSelect, collection.FieldTypeIP:
        return "varchar(100)"
    case collection.FieldTypeMultiSel, collection.FieldTypeCSV:
        return "text[]"
    case collection.FieldTypeHash:
        return "text"
    default:
        return "text"
    }
}
```

### 10. Inspecting the live schema

After creating a collection via the API, verify the table was created:

```bash
psql $DATABASE_URL -c "\d posts"
# Should show all system columns + your custom fields

psql $DATABASE_URL -c "SELECT name, display_name, type FROM collections WHERE tenant_id = '...'"
psql $DATABASE_URL -c "SELECT name, type, db_type FROM fields WHERE collection_name = 'posts'"
```

### 11. Translation shadow table pattern (for i18n)

Translatable fields are stored in a shadow table named `<collection>_translations`. This table is created alongside the main table by `CreateTable` when any field has `Translatable: true`:

```sql
-- Created automatically by CreateTable when translatable fields exist
CREATE TABLE IF NOT EXISTS posts_translations (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    item_id     UUID        NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    tenant_id   UUID        NOT NULL,
    language    VARCHAR(10) NOT NULL,
    -- one column per translatable field:
    title       TEXT,
    excerpt     TEXT,
    UNIQUE (item_id, language)
);
CREATE INDEX IF NOT EXISTS idx_posts_translations_item ON posts_translations (item_id, language);
```

See guide 10 for the full i18n implementation.

## Key Files

- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/postgres.go` — Goose migration runner
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/001_initial_schema.sql` — full system schema
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/002_super_admin_and_storage.sql` — super admin + files table
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — DDL port interface
- `/home/jd/dev/full/cms/backend/internal/core/domain/collection/entity.go` — Field and FieldType definitions

## Next Steps

Proceed to [04 — Auth System](./04-auth-system.md) to implement the full login flow, JWT issuance, refresh tokens, and Logto SSO integration.
