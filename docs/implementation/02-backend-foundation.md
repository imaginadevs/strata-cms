# 02 — Backend Foundation

## Overview

The backend follows a hexagonal architecture (ports and adapters). The domain core contains pure Go types and interfaces with zero external dependencies. Adapters (PostgreSQL, Kafka, S3, HTTP) implement those interfaces. This guide explains the layering and shows step-by-step how to add a new domain entity — using a "Pages" collection as a concrete example.

## Prerequisites

- Guide 01 completed (project running locally).
- `make migrate-up` has been executed at least once.

## Step-by-Step

### 1. Understand the dependency rule

```
HTTP handler → adapter → port interface ← domain entity
                ↑                              ↑
         infrastructure                    core (no deps)
```

Files in `internal/core/` must never import anything from `internal/adapters/` or `internal/infrastructure/`. The compiler enforces this because adapters depend on core, not the other way around.

### 2. Define the domain entity

Create `backend/internal/core/domain/page/entity.go`:

```go
package page

import (
    "github.com/google/uuid"

    domain "github.com/imaginacolombia/cms/internal/core/domain"
)

// Status controls visibility.
type Status string

const (
    StatusDraft     Status = "draft"
    StatusPublished Status = "published"
    StatusArchived  Status = "archived"
)

// Page is a CMS page with a slug and rich-text body.
// It lives in the dynamic collection engine once the user defines it,
// but this type is used for the built-in system page collection.
type Page struct {
    domain.Base                        // id, tenant_id, created_at, updated_at, created_by, updated_by

    Title     string     `json:"title"      db:"title"`
    Slug      string     `json:"slug"       db:"slug"`
    Body      string     `json:"body"       db:"body"`
    Status    Status     `json:"status"     db:"status"`
    SortOrder int        `json:"sort_order" db:"sort_order"`
    ParentID  *uuid.UUID `json:"parent_id"  db:"parent_id"`
}

// NewPage constructs a Page with all required fields.
func NewPage(tenantID uuid.UUID, title, slug string, actorID *uuid.UUID) *Page {
    return &Page{
        Base:   domain.NewBase(tenantID, actorID),
        Title:  title,
        Slug:   slug,
        Status: StatusDraft,
    }
}

// Publish transitions the page to published state.
func (p *Page) Publish(actorID *uuid.UUID) {
    p.Status = StatusPublished
    p.Base.Touch(actorID)
}
```

Note that `domain.Base` (from `internal/core/domain/base.go`) is embedded. It provides `id`, `tenant_id`, `created_at`, `updated_at`, `created_by`, `updated_by` on every entity automatically.

### 3. Define the repository port (interface)

Add the page repository to `backend/internal/core/ports/outbound/repositories.go`:

```go
// ─── Page ─────────────────────────────────────────────────────────────────────

type PageRepository interface {
    Create(ctx context.Context, p *page.Page) error
    GetByID(ctx context.Context, tenantID, id uuid.UUID) (*page.Page, error)
    GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*page.Page, error)
    Update(ctx context.Context, p *page.Page) error
    Delete(ctx context.Context, tenantID, id uuid.UUID) error
    List(ctx context.Context, tenantID uuid.UUID, filter PageFilter) ([]*page.Page, int64, error)
}

type PageFilter struct {
    Status *page.Status
    Search string
    Page   int
    Limit  int
}
```

Add the import at the top of the file:

```go
import (
    // existing imports...
    "github.com/imaginacolombia/cms/internal/core/domain/page"
)
```

### 4. Implement the repository adapter (PostgreSQL)

Create `backend/internal/adapters/postgres/page_repository.go`:

```go
package postgres

import (
    "context"
    "fmt"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/domain/page"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type PageRepository struct {
    db *pgxpool.Pool
}

func NewPageRepository(db *pgxpool.Pool) *PageRepository {
    return &PageRepository{db: db}
}

// Create inserts a new page row. tenant_id is always taken from the entity —
// never from a separate parameter — to prevent cross-tenant writes.
func (r *PageRepository) Create(ctx context.Context, p *page.Page) error {
    const q = `
        INSERT INTO pages (
            id, tenant_id, title, slug, body, status, sort_order, parent_id,
            created_at, updated_at, created_by, updated_by
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8,
            $9, $10, $11, $12
        )`

    _, err := r.db.Exec(ctx, q,
        p.ID, p.TenantID, p.Title, p.Slug, p.Body, p.Status, p.SortOrder, p.ParentID,
        p.CreatedAt, p.UpdatedAt, p.CreatedBy, p.UpdatedBy,
    )
    if err != nil {
        return fmt.Errorf("page.Create: %w", err)
    }
    return nil
}

// GetByID fetches a page by its UUID. The tenant_id check is mandatory.
func (r *PageRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*page.Page, error) {
    const q = `
        SELECT id, tenant_id, title, slug, body, status, sort_order, parent_id,
               created_at, updated_at, created_by, updated_by
        FROM   pages
        WHERE  tenant_id = $1 AND id = $2`

    row := r.db.QueryRow(ctx, q, tenantID, id)
    return scanPage(row)
}

// GetBySlug fetches a page by its slug within a tenant.
func (r *PageRepository) GetBySlug(ctx context.Context, tenantID uuid.UUID, slug string) (*page.Page, error) {
    const q = `
        SELECT id, tenant_id, title, slug, body, status, sort_order, parent_id,
               created_at, updated_at, created_by, updated_by
        FROM   pages
        WHERE  tenant_id = $1 AND slug = $2`

    row := r.db.QueryRow(ctx, q, tenantID, slug)
    return scanPage(row)
}

// Update writes all mutable fields. Immutable fields (id, tenant_id, created_at,
// created_by) are intentionally excluded from the SET clause.
func (r *PageRepository) Update(ctx context.Context, p *page.Page) error {
    const q = `
        UPDATE pages
        SET    title      = $1,
               slug       = $2,
               body       = $3,
               status     = $4,
               sort_order = $5,
               parent_id  = $6,
               updated_at = $7,
               updated_by = $8
        WHERE  tenant_id  = $9 AND id = $10`

    _, err := r.db.Exec(ctx, q,
        p.Title, p.Slug, p.Body, p.Status, p.SortOrder, p.ParentID,
        p.UpdatedAt, p.UpdatedBy,
        p.TenantID, p.ID,
    )
    if err != nil {
        return fmt.Errorf("page.Update: %w", err)
    }
    return nil
}

// Delete is a hard delete. For soft-delete, add a deleted_at column and change
// this to UPDATE pages SET deleted_at = NOW() WHERE ...
func (r *PageRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
    const q = `DELETE FROM pages WHERE tenant_id = $1 AND id = $2`
    _, err := r.db.Exec(ctx, q, tenantID, id)
    return err
}

// List returns a paginated page of results filtered by tenantID.
func (r *PageRepository) List(
    ctx context.Context,
    tenantID uuid.UUID,
    filter outbound.PageFilter,
) ([]*page.Page, int64, error) {
    // Build WHERE clause dynamically
    args := []any{tenantID}
    where := "tenant_id = $1"
    idx := 2

    if filter.Status != nil {
        where += fmt.Sprintf(" AND status = $%d", idx)
        args = append(args, *filter.Status)
        idx++
    }
    if filter.Search != "" {
        where += fmt.Sprintf(" AND (title ILIKE $%d OR slug ILIKE $%d)", idx, idx)
        args = append(args, "%"+filter.Search+"%")
        idx++
    }

    // Total count
    var total int64
    if err := r.db.QueryRow(ctx,
        "SELECT COUNT(*) FROM pages WHERE "+where, args...,
    ).Scan(&total); err != nil {
        return nil, 0, err
    }

    // Paginated rows
    limit := filter.Limit
    if limit <= 0 {
        limit = 25
    }
    offset := (filter.Page - 1) * limit
    if offset < 0 {
        offset = 0
    }

    args = append(args, limit, offset)
    rows, err := r.db.Query(ctx,
        fmt.Sprintf(`
            SELECT id, tenant_id, title, slug, body, status, sort_order, parent_id,
                   created_at, updated_at, created_by, updated_by
            FROM   pages
            WHERE  %s
            ORDER  BY sort_order ASC, created_at DESC
            LIMIT  $%d OFFSET $%d`, where, idx, idx+1),
        args...,
    )
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    var pages []*page.Page
    for rows.Next() {
        p, err := scanPage(rows)
        if err != nil {
            return nil, 0, err
        }
        pages = append(pages, p)
    }
    return pages, total, rows.Err()
}

// scanPage reads a single row into a Page struct.
func scanPage(row pgx.Row) (*page.Page, error) {
    p := &page.Page{}
    err := row.Scan(
        &p.ID, &p.TenantID, &p.Title, &p.Slug, &p.Body,
        &p.Status, &p.SortOrder, &p.ParentID,
        &p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
    )
    if err != nil {
        return nil, fmt.Errorf("scanPage: %w", err)
    }
    return p, nil
}
```

### 5. Create the migration for the pages table

```bash
make migrate-create name=add_pages
```

Edit the generated file in `backend/internal/infrastructure/database/migrations/`:

```sql
-- +goose Up
-- +goose StatementBegin
CREATE TABLE pages (
    id          UUID        PRIMARY KEY DEFAULT uuid_generate_v4(),
    tenant_id   UUID        NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title       VARCHAR(500) NOT NULL,
    slug        VARCHAR(500) NOT NULL,
    body        TEXT        NOT NULL DEFAULT '',
    status      VARCHAR(20) NOT NULL DEFAULT 'draft'
                CHECK (status IN ('draft', 'published', 'archived')),
    sort_order  INTEGER     NOT NULL DEFAULT 0,
    parent_id   UUID        REFERENCES pages(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    updated_by  UUID        REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE (tenant_id, slug)
);

CREATE INDEX idx_pages_tenant ON pages (tenant_id);
CREATE INDEX idx_pages_slug   ON pages (tenant_id, slug);
CREATE INDEX idx_pages_status ON pages (tenant_id, status);

CREATE TRIGGER trg_pages_updated_at
    BEFORE UPDATE ON pages
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS pages;
-- +goose StatementEnd
```

Apply the migration:

```bash
make migrate-up
```

### 6. Write an HTTP handler

Create `backend/internal/adapters/http/rest/pages.go`:

```go
package rest

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
    postgresAdapter "github.com/imaginacolombia/cms/internal/adapters/postgres"
    "github.com/imaginacolombia/cms/internal/core/domain/page"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

type pageHandler struct {
    repo outbound.PageRepository
    bus  events.Bus
    log  *logger.Logger
}

func MountPages(r chi.Router, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
    h := &pageHandler{
        repo: postgresAdapter.NewPageRepository(db),
        bus:  bus,
        log:  log,
    }

    r.Route("/pages", func(r chi.Router) {
        r.Get("/",        h.list)
        r.Post("/",       h.create)
        r.Get("/{id}",   h.getByID)
        r.Patch("/{id}", h.update)
        r.Delete("/{id}", h.delete)
    })
}

func (h *pageHandler) list(w http.ResponseWriter, r *http.Request) {
    tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
    if !ok {
        writeError(w, http.StatusUnauthorized, "tenant not resolved")
        return
    }

    pages, total, err := h.repo.List(r.Context(), tenantID, outbound.PageFilter{Page: 1, Limit: 25})
    if err != nil {
        h.log.Errorw("list pages", "err", err)
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
    writeJSON(w, http.StatusOK, pages)
}

func (h *pageHandler) create(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    userID, _ := httpMiddleware.UserIDFromCtx(r.Context())

    var body struct {
        Title string `json:"title"`
        Slug  string `json:"slug"`
        Body  string `json:"body"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    p := page.NewPage(tenantID, body.Title, body.Slug, &userID)
    p.Body = body.Body

    if err := h.repo.Create(r.Context(), p); err != nil {
        h.log.Errorw("create page", "err", err)
        writeError(w, http.StatusInternalServerError, "internal error")
        return
    }

    _ = h.bus.Publish(r.Context(), events.Event{
        Type:       events.EventAfterCreate,
        Collection: "pages",
        TenantID:   tenantID,
        UserID:     &userID,
        ItemID:     strPtr(p.ID.String()),
        Payload:    map[string]any{"title": p.Title, "slug": p.Slug},
    })

    writeJSON(w, http.StatusCreated, p)
}

func (h *pageHandler) getByID(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    id, err := uuid.Parse(chi.URLParam(r, "id"))
    if err != nil {
        writeError(w, http.StatusBadRequest, "invalid id")
        return
    }

    p, err := h.repo.GetByID(r.Context(), tenantID, id)
    if err != nil {
        writeError(w, http.StatusNotFound, "not found")
        return
    }
    writeJSON(w, http.StatusOK, p)
}
```

### 7. Wire everything in main.go

In `backend/cmd/server/main.go`, after creating the bus and before starting the server, inject the new handler. In practice this is done by updating `NewRouter` in `backend/internal/adapters/http/router.go`:

```go
// Inside the protected group in router.go:
rest.MountPages(r, db, bus, log)
```

### 8. Test the endpoint

```bash
# Get a JWT first (see guide 04)
TOKEN="<jwt from login>"

# Create a page
curl -X POST http://localhost:8080/api/pages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: 00000000-0000-0000-0000-000000000001" \
  -d '{"title":"Home","slug":"home","body":"<p>Hello</p>"}'

# List pages
curl http://localhost:8080/api/pages \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: 00000000-0000-0000-0000-000000000001"
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/core/domain/base.go` — shared Base struct
- `/home/jd/dev/full/cms/backend/internal/core/domain/collection/entity.go` — collection entity (reference)
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — all repository interfaces
- `/home/jd/dev/full/cms/backend/internal/adapters/http/router.go` — chi router wiring
- `/home/jd/dev/full/cms/backend/internal/adapters/http/rest/auth.go` — REST handler pattern to follow
- `/home/jd/dev/full/cms/backend/cmd/server/main.go` — application entry point

## Next Steps

Proceed to [03 — Database Schema](./03-database-schema.md) to learn how Goose migrations work and how dynamic collection tables are created at runtime.
