# 06 — Collection Engine

## Overview

The collection engine is the heart of the CMS. It allows administrators to define arbitrary data models (collections) through the API, which are then materialized as real PostgreSQL tables. Each collection has a type (`base` or `auth`), a set of field definitions stored in the `fields` table, and a live table whose schema is managed via DDL adapters. This guide covers the full lifecycle: create a collection, add fields, handle the two collection types, and understand system field injection.

## Prerequisites

- Guides 03 and 05 completed (migrations applied, tenant middleware wired).
- Familiarity with `internal/core/domain/collection/entity.go`.

## Step-by-Step

### 1. Data model overview

```
collections (metadata row)
    ├── fields (column definitions)
    │       └── field_translations (display name translations)
    └── relations (foreign key relationships)

Live PostgreSQL table (created by DDL adapter):
    posts
    ├── id           (system field — always injected)
    ├── tenant_id    (system field — always injected)
    ├── title        (user-defined: FieldTypeString)
    ├── body         (user-defined: FieldTypeRichText)
    ├── published_at (user-defined: FieldTypeDateTime)
    ├── created_at   (system field)
    ├── updated_at   (system field)
    ├── created_by   (system field)
    └── updated_by   (system field)
```

For `auth` collections, additional system fields are injected:

```
users_collection (type = "auth")
    ├── id, tenant_id, created_at, updated_at, created_by, updated_by
    ├── username       (auth system field)
    ├── password_hash  (auth system field — hidden, never returned by API)
    ├── last_login     (auth system field)
    ├── status         (auth system field — active/inactive/suspended)
    └── email          (user-defined example field)
```

### 2. Creating a collection via the API

```bash
TOKEN="<admin JWT>"
TENANT="00000000-0000-0000-0000-000000000001"

# Create a "posts" collection (base type)
curl -X POST http://localhost:8080/api/collections \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "posts",
    "display_name": "Blog Posts",
    "type": "base",
    "icon": "article",
    "color": "#10B981"
  }'
```

Behind the scenes the handler calls `CollectionRepository.Create()` (inserts the metadata row) and then `CollectionRepository.CreateTable()` (executes the DDL).

### 3. Collection repository implementation

Create `backend/internal/adapters/postgres/collection_repository.go`:

```go
package postgres

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/domain/collection"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type CollectionRepository struct {
    db *pgxpool.Pool
}

func NewCollectionRepository(db *pgxpool.Pool) *CollectionRepository {
    return &CollectionRepository{db: db}
}

func (r *CollectionRepository) Create(ctx context.Context, c *collection.Collection) error {
    metaJSON, _ := json.Marshal(c.Meta)
    _, err := r.db.Exec(ctx, `
        INSERT INTO collections (
            id, tenant_id, name, display_name, type, icon, color, note,
            is_system, is_hidden, is_singleton, sort_field, meta,
            created_at, updated_at, created_by, updated_by
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
        c.ID, c.TenantID, c.Name, c.DisplayName, c.Type, c.Icon, c.Color, c.Note,
        c.IsSystem, c.IsHidden, c.IsSingleton, c.SortField, metaJSON,
        c.CreatedAt, c.UpdatedAt, c.CreatedBy, c.UpdatedBy,
    )
    return err
}

func (r *CollectionRepository) GetByName(
    ctx context.Context,
    tenantID uuid.UUID,
    name string,
) (*collection.Collection, error) {
    return r.scanOne(ctx, `
        SELECT id, tenant_id, name, display_name, type, icon, color, note,
               is_system, is_hidden, is_singleton, sort_field, meta,
               created_at, updated_at, created_by, updated_by
        FROM   collections
        WHERE  tenant_id = $1 AND name = $2`, tenantID, name)
}

func (r *CollectionRepository) GetByID(
    ctx context.Context,
    tenantID, id uuid.UUID,
) (*collection.Collection, error) {
    return r.scanOne(ctx, `
        SELECT id, tenant_id, name, display_name, type, icon, color, note,
               is_system, is_hidden, is_singleton, sort_field, meta,
               created_at, updated_at, created_by, updated_by
        FROM   collections
        WHERE  tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *CollectionRepository) Update(ctx context.Context, c *collection.Collection) error {
    metaJSON, _ := json.Marshal(c.Meta)
    _, err := r.db.Exec(ctx, `
        UPDATE collections
        SET    display_name = $1, icon = $2, color = $3, note = $4,
               is_hidden = $5, is_singleton = $6, sort_field = $7, meta = $8,
               updated_at = NOW(), updated_by = $9
        WHERE  tenant_id = $10 AND name = $11`,
        c.DisplayName, c.Icon, c.Color, c.Note,
        c.IsHidden, c.IsSingleton, c.SortField, metaJSON,
        c.UpdatedBy, c.TenantID, c.Name,
    )
    return err
}

func (r *CollectionRepository) Delete(ctx context.Context, tenantID uuid.UUID, name string) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM collections WHERE tenant_id = $1 AND name = $2 AND is_system = FALSE`,
        tenantID, name,
    )
    return err
}

func (r *CollectionRepository) List(
    ctx context.Context,
    tenantID uuid.UUID,
    includeHidden bool,
) ([]*collection.Collection, error) {
    q := `
        SELECT id, tenant_id, name, display_name, type, icon, color, note,
               is_system, is_hidden, is_singleton, sort_field, meta,
               created_at, updated_at, created_by, updated_by
        FROM   collections
        WHERE  tenant_id = $1`
    if !includeHidden {
        q += ` AND is_hidden = FALSE`
    }
    q += ` ORDER BY display_name ASC`

    rows, err := r.db.Query(ctx, q, tenantID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var cols []*collection.Collection
    for rows.Next() {
        c, err := r.scan(rows)
        if err != nil {
            return nil, err
        }
        cols = append(cols, c)
    }
    return cols, rows.Err()
}

func (r *CollectionRepository) scanOne(ctx context.Context, q string, args ...any) (*collection.Collection, error) {
    return r.scan(r.db.QueryRow(ctx, q, args...))
}

func (r *CollectionRepository) scan(row interface{ Scan(dest ...any) error }) (*collection.Collection, error) {
    c := &collection.Collection{}
    var metaJSON []byte
    err := row.Scan(
        &c.ID, &c.TenantID, &c.Name, &c.DisplayName, &c.Type,
        &c.Icon, &c.Color, &c.Note,
        &c.IsSystem, &c.IsHidden, &c.IsSingleton, &c.SortField, &metaJSON,
        &c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.UpdatedBy,
    )
    if err != nil {
        return nil, err
    }
    _ = json.Unmarshal(metaJSON, &c.Meta)
    return c, nil
}
```

### 4. Field repository implementation

Create `backend/internal/adapters/postgres/field_repository.go`:

```go
package postgres

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/domain/collection"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type FieldRepository struct {
    db *pgxpool.Pool
}

func NewFieldRepository(db *pgxpool.Pool) *FieldRepository {
    return &FieldRepository{db: db}
}

func (r *FieldRepository) Create(ctx context.Context, f *collection.Field) error {
    optionsJSON, _ := json.Marshal(f.Options)
    validationJSON, _ := json.Marshal(f.Validation)
    defaultJSON, _ := json.Marshal(f.DefaultValue)

    _, err := r.db.Exec(ctx, `
        INSERT INTO fields (
            id, tenant_id, collection_name, name, display_name, type, db_type,
            required, unique_field, readonly, hidden, translatable,
            default_value, validation, options, sort, is_system,
            created_at, updated_at, created_by, updated_by
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
        f.ID, f.TenantID, f.CollectionName, f.Name, f.DisplayName,
        f.Type, f.DBType,
        f.Required, f.Unique, f.Readonly, f.Hidden, f.Translatable,
        defaultJSON, validationJSON, optionsJSON,
        f.Sort, f.IsSystem,
        f.CreatedAt, f.UpdatedAt, f.CreatedBy, f.UpdatedBy,
    )
    return err
}

func (r *FieldRepository) ListByCollection(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName string,
) ([]*collection.Field, error) {
    rows, err := r.db.Query(ctx, `
        SELECT id, tenant_id, collection_name, name, display_name, type, db_type,
               required, unique_field, readonly, hidden, translatable,
               default_value, validation, options, sort, is_system,
               created_at, updated_at, created_by, updated_by
        FROM   fields
        WHERE  tenant_id = $1 AND collection_name = $2
        ORDER  BY sort ASC, name ASC`,
        tenantID, collectionName,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var fields []*collection.Field
    for rows.Next() {
        f, err := scanField(rows)
        if err != nil {
            return nil, err
        }
        fields = append(fields, f)
    }
    return fields, rows.Err()
}

func (r *FieldRepository) Delete(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName, fieldName string,
) error {
    _, err := r.db.Exec(ctx, `
        DELETE FROM fields
        WHERE  tenant_id = $1 AND collection_name = $2 AND name = $3 AND is_system = FALSE`,
        tenantID, collectionName, fieldName,
    )
    return err
}

func scanField(row interface{ Scan(dest ...any) error }) (*collection.Field, error) {
    f := &collection.Field{}
    var optJSON, validJSON, defJSON []byte
    err := row.Scan(
        &f.ID, &f.TenantID, &f.CollectionName, &f.Name, &f.DisplayName,
        &f.Type, &f.DBType,
        &f.Required, &f.Unique, &f.Readonly, &f.Hidden, &f.Translatable,
        &defJSON, &validJSON, &optJSON,
        &f.Sort, &f.IsSystem,
        &f.CreatedAt, &f.UpdatedAt, &f.CreatedBy, &f.UpdatedBy,
    )
    if err != nil {
        return nil, err
    }
    _ = json.Unmarshal(optJSON, &f.Options)
    _ = json.Unmarshal(validJSON, &f.Validation)
    _ = json.Unmarshal(defJSON, &f.DefaultValue)
    return f, nil
}
```

### 5. Full collection creation handler

Create `backend/internal/adapters/http/rest/collections.go`:

```go
package rest

import (
    "encoding/json"
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
    postgresAdapter "github.com/imaginacolombia/cms/internal/adapters/postgres"
    "github.com/imaginacolombia/cms/internal/core/domain/collection"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

type collectionHandler struct {
    collRepo  outbound.CollectionRepository
    fieldRepo outbound.FieldRepository
    bus       events.Bus
    log       *logger.Logger
}

func MountCollections(r chi.Router, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
    h := &collectionHandler{
        collRepo:  postgresAdapter.NewCollectionRepository(db),
        fieldRepo: postgresAdapter.NewFieldRepository(db),
        bus:       bus,
        log:       log,
    }

    r.Route("/collections", func(r chi.Router) {
        r.Get("/",                            h.list)
        r.Post("/",                           h.create)
        r.Get("/{name}",                     h.getByName)
        r.Patch("/{name}",                   h.update)
        r.Delete("/{name}",                  h.delete)
        r.Post("/{name}/fields",             h.addField)
        r.Patch("/{name}/fields/{field}",    h.updateField)
        r.Delete("/{name}/fields/{field}",   h.deleteField)
    })
}

type createCollectionRequest struct {
    Name        string                     `json:"name"`
    DisplayName string                     `json:"display_name"`
    Type        collection.CollectionType  `json:"type"`
    Icon        *string                    `json:"icon"`
    Color       *string                    `json:"color"`
    Note        *string                    `json:"note"`
    Fields      []createFieldRequest       `json:"fields"`
}

type createFieldRequest struct {
    Name         string              `json:"name"`
    DisplayName  string              `json:"display_name"`
    Type         collection.FieldType `json:"type"`
    Required     bool                `json:"required"`
    Unique       bool                `json:"unique"`
    Translatable bool                `json:"translatable"`
    Options      map[string]any      `json:"options"`
}

func (h *collectionHandler) create(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

    var req createCollectionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }

    c := collection.NewCollection(tenantID, req.Name, req.DisplayName, req.Type, &actorID)
    c.Icon = req.Icon
    c.Color = req.Color
    c.Note = req.Note

    // Build field list (user-defined fields only; system fields are injected by DDL adapter)
    for i, fr := range req.Fields {
        dbType := FieldTypeToDBType(fr.Type)
        f := collection.Field{
            Base:           domain.NewBase(tenantID, &actorID),
            CollectionName: c.Name,
            Name:           fr.Name,
            DisplayName:    fr.DisplayName,
            Type:           fr.Type,
            DBType:         dbType,
            Required:       fr.Required,
            Unique:         fr.Unique,
            Translatable:   fr.Translatable,
            Options:        fr.Options,
            Sort:           i,
        }
        c.Fields = append(c.Fields, f)
    }

    // 1. Persist metadata
    if err := h.collRepo.Create(r.Context(), c); err != nil {
        h.log.Errorw("create collection metadata", "err", err)
        writeError(w, http.StatusConflict, "collection name already exists or DB error")
        return
    }

    // 2. Persist field definitions
    for i := range c.Fields {
        if err := h.fieldRepo.Create(r.Context(), &c.Fields[i]); err != nil {
            h.log.Errorw("create field", "field", c.Fields[i].Name, "err", err)
        }
    }

    // 3. Execute DDL — create the physical table
    if err := h.collRepo.CreateTable(r.Context(), tenantID, c); err != nil {
        h.log.Errorw("DDL CreateTable", "collection", c.Name, "err", err)
        writeError(w, http.StatusInternalServerError, "could not create table: "+err.Error())
        return
    }

    // 4. Publish schema event
    _ = h.bus.Publish(r.Context(), events.Event{
        Type:       events.EventCollectionCreated,
        Collection: c.Name,
        TenantID:   tenantID,
        UserID:     &actorID,
        Payload:    map[string]any{"type": c.Type, "name": c.Name},
    })

    writeJSON(w, http.StatusCreated, c)
}

func (h *collectionHandler) addField(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
    collectionName := chi.URLParam(r, "name")

    var req createFieldRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }

    f := &collection.Field{
        Base:           domain.NewBase(tenantID, &actorID),
        CollectionName: collectionName,
        Name:           req.Name,
        DisplayName:    req.DisplayName,
        Type:           req.Type,
        DBType:         FieldTypeToDBType(req.Type),
        Required:       req.Required,
        Unique:         req.Unique,
        Translatable:   req.Translatable,
        Options:        req.Options,
    }

    // 1. Persist field metadata
    if err := h.fieldRepo.Create(r.Context(), f); err != nil {
        writeError(w, http.StatusConflict, "field already exists")
        return
    }

    // 2. ALTER TABLE — add the column
    if err := h.collRepo.AddColumn(r.Context(), tenantID, f); err != nil {
        writeError(w, http.StatusInternalServerError, "DDL error: "+err.Error())
        return
    }

    _ = h.bus.Publish(r.Context(), events.Event{
        Type:       events.EventFieldCreated,
        Collection: collectionName,
        TenantID:   tenantID,
        UserID:     &actorID,
    })

    writeJSON(w, http.StatusCreated, f)
}

func (h *collectionHandler) deleteField(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    collectionName := chi.URLParam(r, "name")
    fieldName := chi.URLParam(r, "field")

    // 1. Check field is not a system field
    f, err := h.fieldRepo.GetByName(r.Context(), tenantID, collectionName, fieldName)
    if err != nil {
        writeError(w, http.StatusNotFound, "field not found")
        return
    }
    if f.IsSystem {
        writeError(w, http.StatusForbidden, "cannot delete system fields")
        return
    }

    // 2. Remove metadata
    if err := h.fieldRepo.Delete(r.Context(), tenantID, collectionName, fieldName); err != nil {
        writeError(w, http.StatusInternalServerError, "error deleting field")
        return
    }

    // 3. ALTER TABLE DROP COLUMN
    if err := h.collRepo.DropColumn(r.Context(), tenantID, collectionName, fieldName); err != nil {
        writeError(w, http.StatusInternalServerError, "DDL error: "+err.Error())
        return
    }

    w.WriteHeader(http.StatusNoContent)
}

func (h *collectionHandler) delete(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    name := chi.URLParam(r, "name")

    // Prevent deletion of system collections
    c, err := h.collRepo.GetByName(r.Context(), tenantID, name)
    if err != nil {
        writeError(w, http.StatusNotFound, "collection not found")
        return
    }
    if c.IsSystem {
        writeError(w, http.StatusForbidden, "cannot delete system collections")
        return
    }

    // 1. Drop physical table (CASCADE removes dependent objects)
    if err := h.collRepo.DropTable(r.Context(), tenantID, name); err != nil {
        writeError(w, http.StatusInternalServerError, "DDL error: "+err.Error())
        return
    }

    // 2. Remove metadata (fields cascade via FK)
    if err := h.collRepo.Delete(r.Context(), tenantID, name); err != nil {
        writeError(w, http.StatusInternalServerError, "error deleting collection metadata")
        return
    }

    _ = h.bus.Publish(r.Context(), events.Event{
        Type:       events.EventCollectionDeleted,
        Collection: name,
        TenantID:   tenantID,
    })

    w.WriteHeader(http.StatusNoContent)
}
```

### 6. Auth collection type — system field injection

When `collection.Type == CollectionTypeAuth`, `AuthSystemFields()` is called before user fields:

```go
// From internal/core/domain/collection/entity.go
func AuthSystemFields(collectionName string, tenantID interface{}) []Field {
    return []Field{
        {Name: "username",      Type: FieldTypeString,   DBType: "varchar(255)", Required: true, Unique: true, IsSystem: true},
        {Name: "password_hash", Type: FieldTypeHash,     DBType: "text",         Hidden: true,               IsSystem: true},
        {Name: "last_login",    Type: FieldTypeDateTime, DBType: "timestamptz",                               IsSystem: true},
        {Name: "status",        Type: FieldTypeSelect,   DBType: "varchar(50)",  Required: true,              IsSystem: true,
            DefaultValue: "active",
            Options: map[string]any{
                "choices": []map[string]string{
                    {"value": "active",    "label": "Active"},
                    {"value": "inactive",  "label": "Inactive"},
                    {"value": "suspended", "label": "Suspended"},
                },
            },
        },
    }
}
```

The DDL `CreateTable` function calls this automatically (see guide 03, step 8). The auth collection also gets registered with the authentication engine so that `POST /api/auth/login` can use it as the user source.

### 7. Verify a created collection

```bash
# List all collections for the tenant
curl http://localhost:8080/api/collections \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"

# Verify the physical table exists
psql $DATABASE_URL -c "\d posts"

# Verify field metadata
psql $DATABASE_URL -c "
  SELECT name, type, db_type, required, translatable
  FROM   fields
  WHERE  collection_name = 'posts'
  ORDER  BY sort"
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/core/domain/collection/entity.go` — Collection, Field, FieldType, AuthSystemFields
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — CollectionRepository + FieldRepository interfaces
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/001_initial_schema.sql` — collections, fields, relations DDL

## Next Steps

Proceed to [07 — Permissions](./07-permissions.md) to implement role-based access control, field-level permissions, row-level filters, and system roles.
