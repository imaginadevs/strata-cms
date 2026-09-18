# 08 — API Layer

## Overview

The API layer has three protocols: REST (Chi), GraphQL (gqlgen), and Server-Sent Events for real-time push. REST item endpoints are dynamic — the same handler serves every collection by reading the collection name from the URL. GraphQL schemas are generated at runtime from collection definitions. SSE replaces WebSocket for simplicity; clients connect once and receive a stream of JSON events. This guide implements a full "posts" collection REST resource, the GraphQL endpoint, and the SSE endpoint.

## Prerequisites

- Guides 06 and 07 completed (collections and permissions working).
- gqlgen is listed in `go.mod` (`github.com/99designs/gqlgen v0.17.49`).

## Step-by-Step

### 1. Dynamic item repository implementation

The `ItemRepository` operates on any collection without knowing its schema at compile time. It builds queries from column names at runtime.

Create `backend/internal/adapters/postgres/item_repository.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "strings"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type ItemRepository struct {
    db        *pgxpool.Pool
    permRepo  outbound.PermissionRepository
    fieldRepo outbound.FieldRepository
}

func NewItemRepository(db *pgxpool.Pool, permRepo outbound.PermissionRepository, fieldRepo outbound.FieldRepository) *ItemRepository {
    return &ItemRepository{db: db, permRepo: permRepo, fieldRepo: fieldRepo}
}

func (r *ItemRepository) Create(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName string,
    data map[string]any,
) (map[string]any, error) {
    // Always inject system fields
    id := uuid.New()
    data["id"] = id
    data["tenant_id"] = tenantID

    cols := make([]string, 0, len(data))
    placeholders := make([]string, 0, len(data))
    args := make([]any, 0, len(data))
    i := 1

    for col, val := range data {
        cols = append(cols, pgIdentifier(col))
        placeholders = append(placeholders, fmt.Sprintf("$%d", i))
        args = append(args, val)
        i++
    }

    q := fmt.Sprintf(
        "INSERT INTO %s (%s) VALUES (%s) RETURNING *",
        pgIdentifier(collectionName),
        strings.Join(cols, ", "),
        strings.Join(placeholders, ", "),
    )

    rows, err := r.db.Query(ctx, q, args...)
    if err != nil {
        return nil, fmt.Errorf("item.Create: %w", err)
    }
    defer rows.Close()

    if rows.Next() {
        return rowToMap(rows)
    }
    return data, nil
}

func (r *ItemRepository) GetByID(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName, id string,
    filter outbound.QueryFilter,
) (map[string]any, error) {
    q := fmt.Sprintf(
        "SELECT * FROM %s WHERE tenant_id = $1 AND id = $2",
        pgIdentifier(collectionName),
    )
    rows, err := r.db.Query(ctx, q, tenantID, id)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    if rows.Next() {
        item, err := rowToMap(rows)
        if err != nil {
            return nil, err
        }
        // Apply language overlay for i18n if lang is specified
        if filter.Lang != "" {
            applyTranslations(ctx, r.db, collectionName, id, filter.Lang, item)
        }
        return item, nil
    }
    return nil, fmt.Errorf("item not found")
}

func (r *ItemRepository) List(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName string,
    filter outbound.QueryFilter,
) ([]map[string]any, int64, error) {
    args := []any{tenantID}
    where := "tenant_id = $1"
    i := 2

    // Search
    if filter.Search != "" {
        // Full-text search uses pg_trgm for fuzzy matching
        where += fmt.Sprintf(" AND to_tsvector('simple', *::text) @@ plainto_tsquery('simple', $%d)", i)
        args = append(args, filter.Search)
        i++
    }

    // User-supplied filter (passed as map from REST query params)
    filterSQL, filterArgs := buildFilterFromMap(filter.Filter, i-1)
    if filterSQL != "" {
        where += " AND " + filterSQL
        args = append(args, filterArgs...)
        i += len(filterArgs)
    }

    // Count
    var total int64
    _ = r.db.QueryRow(ctx,
        fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", pgIdentifier(collectionName), where),
        args...,
    ).Scan(&total)

    // Sort
    orderBy := "created_at DESC"
    if len(filter.Sort) > 0 {
        parts := make([]string, len(filter.Sort))
        for j, s := range filter.Sort {
            dir := "ASC"
            if s.Desc {
                dir = "DESC"
            }
            parts[j] = pgIdentifier(s.Field) + " " + dir
        }
        orderBy = strings.Join(parts, ", ")
    }

    // Pagination
    limit := filter.Limit
    if limit <= 0 {
        limit = 25
    }
    if limit > 1000 {
        limit = 1000
    }
    offset := (filter.Page - 1) * limit
    if offset < 0 {
        offset = 0
    }

    args = append(args, limit, offset)
    q := fmt.Sprintf(
        "SELECT * FROM %s WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d",
        pgIdentifier(collectionName), where, orderBy, i, i+1,
    )

    rows, err := r.db.Query(ctx, q, args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    var items []map[string]any
    for rows.Next() {
        item, err := rowToMap(rows)
        if err != nil {
            return nil, 0, err
        }
        items = append(items, item)
    }
    return items, total, rows.Err()
}

func (r *ItemRepository) Update(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName, id string,
    data map[string]any,
) (map[string]any, error) {
    sets := make([]string, 0, len(data))
    args := []any{}
    i := 1

    for col, val := range data {
        if col == "id" || col == "tenant_id" || col == "created_at" || col == "created_by" {
            continue // immutable fields
        }
        sets = append(sets, fmt.Sprintf("%s = $%d", pgIdentifier(col), i))
        args = append(args, val)
        i++
    }

    args = append(args, tenantID, id)
    q := fmt.Sprintf(
        "UPDATE %s SET %s, updated_at = NOW() WHERE tenant_id = $%d AND id = $%d RETURNING *",
        pgIdentifier(collectionName),
        strings.Join(sets, ", "),
        i, i+1,
    )

    rows, err := r.db.Query(ctx, q, args...)
    if err != nil {
        return nil, fmt.Errorf("item.Update: %w", err)
    }
    defer rows.Close()

    if rows.Next() {
        return rowToMap(rows)
    }
    return nil, fmt.Errorf("item not found after update")
}

func (r *ItemRepository) Delete(
    ctx context.Context,
    tenantID uuid.UUID,
    collectionName, id string,
) error {
    _, err := r.db.Exec(ctx,
        fmt.Sprintf("DELETE FROM %s WHERE tenant_id = $1 AND id = $2", pgIdentifier(collectionName)),
        tenantID, id,
    )
    return err
}

// rowToMap converts a pgx.Rows result into map[string]any.
func rowToMap(rows interface {
    FieldDescriptions() []pgconn.FieldDescription
    Values() ([]any, error)
}) (map[string]any, error) {
    fields := rows.FieldDescriptions()
    values, err := rows.Values()
    if err != nil {
        return nil, err
    }
    m := make(map[string]any, len(fields))
    for i, f := range fields {
        m[string(f.Name)] = values[i]
    }
    return m, nil
}
```

### 2. REST items handler with pagination

Create `backend/internal/adapters/http/rest/items.go`:

```go
package rest

import (
    "encoding/json"
    "fmt"
    "net/http"
    "strconv"
    "strings"

    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
    postgresAdapter "github.com/imaginacolombia/cms/internal/adapters/postgres"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

type itemHandler struct {
    itemRepo outbound.ItemRepository
    bus      events.Bus
    log      *logger.Logger
}

func MountItems(r chi.Router, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
    permRepo := postgresAdapter.NewPermissionRepository(db)
    fieldRepo := postgresAdapter.NewFieldRepository(db)

    h := &itemHandler{
        itemRepo: postgresAdapter.NewItemRepository(db, permRepo, fieldRepo),
        bus:      bus,
        log:      log,
    }

    // All collection routes share the same handler — collection name from URL
    r.Route("/items/{collection}", func(r chi.Router) {
        r.Get("/",        h.list)
        r.Post("/",       h.create)
        r.Get("/{id}",   h.getByID)
        r.Patch("/{id}", h.update)
        r.Delete("/{id}", h.delete)
    })
}

func (h *itemHandler) list(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    collectionName := chi.URLParam(r, "collection")

    filter := parseQueryFilter(r)

    items, total, err := h.itemRepo.List(r.Context(), tenantID, collectionName, filter)
    if err != nil {
        h.log.Errorw("list items", "collection", collectionName, "err", err)
        writeError(w, http.StatusInternalServerError, "error listing items")
        return
    }

    // Pagination headers (RFC 5988 style)
    w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
    writeJSON(w, http.StatusOK, map[string]any{
        "data":  items,
        "total": total,
        "page":  filter.Page,
        "limit": filter.Limit,
    })
}

func (h *itemHandler) create(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    userID, _ := httpMiddleware.UserIDFromCtx(r.Context())
    collectionName := chi.URLParam(r, "collection")

    var data map[string]any
    if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }

    // Fire before.create hook — if it returns error, abort
    if err := h.bus.Publish(r.Context(), events.Event{
        Type:       events.EventBeforeCreate,
        Collection: collectionName,
        TenantID:   tenantID,
        UserID:     &userID,
        Payload:    data,
    }); err != nil {
        writeError(w, http.StatusBadRequest, "hook rejected: "+err.Error())
        return
    }

    item, err := h.itemRepo.Create(r.Context(), tenantID, collectionName, data)
    if err != nil {
        h.log.Errorw("create item", "collection", collectionName, "err", err)
        writeError(w, http.StatusInternalServerError, "error creating item")
        return
    }

    // Fire after.create hook (non-blocking side effects)
    go h.bus.Publish(r.Context(), events.Event{  //nolint:errcheck
        Type:       events.EventAfterCreate,
        Collection: collectionName,
        TenantID:   tenantID,
        UserID:     &userID,
        Payload:    item,
    })

    writeJSON(w, http.StatusCreated, item)
}

// parseQueryFilter extracts filter, sort, pagination, and lang from query params.
//
// Examples:
//   GET /items/posts?filter[status][_eq]=published&sort=-created_at&page=2&limit=10&lang=es
func parseQueryFilter(r *http.Request) outbound.QueryFilter {
    q := r.URL.Query()

    filter := outbound.QueryFilter{
        Filter: make(map[string]any),
        Lang:   q.Get("lang"),
        Search: q.Get("search"),
    }

    // Pagination
    filter.Page, _ = strconv.Atoi(q.Get("page"))
    if filter.Page < 1 {
        filter.Page = 1
    }
    filter.Limit, _ = strconv.Atoi(q.Get("limit"))
    if filter.Limit < 1 {
        filter.Limit = 25
    }

    // Sort: comma-separated, prefix "-" for DESC
    // e.g. sort=-published_at,title
    if sortParam := q.Get("sort"); sortParam != "" {
        for _, part := range strings.Split(sortParam, ",") {
            part = strings.TrimSpace(part)
            if strings.HasPrefix(part, "-") {
                filter.Sort = append(filter.Sort, outbound.SortField{Field: part[1:], Desc: true})
            } else {
                filter.Sort = append(filter.Sort, outbound.SortField{Field: part, Desc: false})
            }
        }
    }

    // Filter: filter[field][_operator]=value
    for key, vals := range q {
        if !strings.HasPrefix(key, "filter[") {
            continue
        }
        // Parse filter[status][_eq] → field=status, op=_eq
        parts := strings.Split(strings.TrimPrefix(strings.TrimSuffix(key, "]"), "filter["), "][")
        if len(parts) == 2 && len(vals) > 0 {
            field, op := parts[0], parts[1]
            if _, exists := filter.Filter[field]; !exists {
                filter.Filter[field] = map[string]any{}
            }
            filter.Filter[field].(map[string]any)[op] = vals[0]
        }
    }

    return filter
}
```

### 3. GraphQL schema generation from collection definitions

The GraphQL schema is generated dynamically when the server starts (or when collections change). The generated schema is fed into gqlgen's runtime.

Create `backend/internal/adapters/http/graphql/schema_generator.go`:

```go
package graphql

import (
    "context"
    "fmt"
    "strings"

    "github.com/google/uuid"

    "github.com/imaginacolombia/cms/internal/core/domain/collection"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// GenerateSchema builds a GraphQL SDL string from collection definitions.
// Call this on startup and whenever a collection/field changes.
func GenerateSchema(
    ctx context.Context,
    tenantID uuid.UUID,
    collRepo outbound.CollectionRepository,
    fieldRepo outbound.FieldRepository,
) (string, error) {
    collections, err := collRepo.List(ctx, tenantID, false)
    if err != nil {
        return "", err
    }

    var sb strings.Builder

    // Scalar declarations
    sb.WriteString(`
scalar UUID
scalar DateTime
scalar JSON

type Query {
`)

    for _, c := range collections {
        typeName := toPascalCase(c.Name)
        sb.WriteString(fmt.Sprintf("  %s(id: UUID!): %s\n", c.Name+"Item", typeName))
        sb.WriteString(fmt.Sprintf("  %s(filter: JSON, sort: String, page: Int, limit: Int): %sList\n",
            c.Name+"Items", typeName))
    }
    sb.WriteString("}\n\n")

    sb.WriteString("type Mutation {\n")
    for _, c := range collections {
        typeName := toPascalCase(c.Name)
        sb.WriteString(fmt.Sprintf("  create%s(data: JSON!): %s\n", typeName, typeName))
        sb.WriteString(fmt.Sprintf("  update%s(id: UUID!, data: JSON!): %s\n", typeName, typeName))
        sb.WriteString(fmt.Sprintf("  delete%s(id: UUID!): Boolean\n", typeName))
    }
    sb.WriteString("}\n\n")

    // Type definitions
    for _, c := range collections {
        typeName := toPascalCase(c.Name)
        fields, _ := fieldRepo.ListByCollection(ctx, tenantID, c.Name)

        // List wrapper
        sb.WriteString(fmt.Sprintf("type %sList {\n  data: [%s!]!\n  total: Int!\n}\n\n", typeName, typeName))

        sb.WriteString(fmt.Sprintf("type %s {\n", typeName))
        // System fields
        sb.WriteString("  id: UUID!\n  tenant_id: UUID!\n  created_at: DateTime!\n  updated_at: DateTime!\n")

        for _, f := range fields {
            if f.Hidden {
                continue
            }
            gqlType := fieldTypeToGraphQL(f.Type, f.Required)
            sb.WriteString(fmt.Sprintf("  %s: %s\n", f.Name, gqlType))
        }
        sb.WriteString("}\n\n")
    }

    return sb.String(), nil
}

func fieldTypeToGraphQL(ft collection.FieldType, required bool) string {
    var base string
    switch ft {
    case collection.FieldTypeString, collection.FieldTypeText,
         collection.FieldTypeRichText, collection.FieldTypeSelect,
         collection.FieldTypeHash, collection.FieldTypeCSV:
        base = "String"
    case collection.FieldTypeInteger:
        base = "Int"
    case collection.FieldTypeFloat, collection.FieldTypeDecimal:
        base = "Float"
    case collection.FieldTypeBoolean:
        base = "Boolean"
    case collection.FieldTypeDate, collection.FieldTypeDateTime,
         collection.FieldTypeTime, collection.FieldTypeTimestamp:
        base = "DateTime"
    case collection.FieldTypeUUID, collection.FieldTypeRelation,
         collection.FieldTypeFile:
        base = "UUID"
    case collection.FieldTypeJSON, collection.FieldTypeFiles,
         collection.FieldTypeMultiSel:
        base = "JSON"
    default:
        base = "String"
    }

    if required {
        return base + "!"
    }
    return base
}

func toPascalCase(s string) string {
    parts := strings.Split(s, "_")
    for i, p := range parts {
        if len(p) > 0 {
            parts[i] = strings.ToUpper(p[:1]) + p[1:]
        }
    }
    return strings.Join(parts, "")
}
```

Mount the GraphQL endpoint (using gqlgen's handler):

```go
// backend/internal/adapters/http/rest/graphql.go
package rest

import (
    "net/http"

    "github.com/99designs/gqlgen/graphql/handler"
    "github.com/99designs/gqlgen/graphql/playground"
    "github.com/go-chi/chi/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountGraphQL(r chi.Router, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
    // Build the executable schema using gqlgen
    // The schema string comes from GenerateSchema() at startup
    // For now mount the playground at /graphql and the API at /graphql/query

    r.Handle("/graphql/query", buildGraphQLHandler(db, bus, log))
    r.Get("/graphql", playground.Handler("CMS GraphQL", "/graphql/query"))
}

func buildGraphQLHandler(db *pgxpool.Pool, bus events.Bus, log *logger.Logger) http.Handler {
    // Wire in your gqlgen generated schema + resolvers
    // See: https://gqlgen.com/getting-started/
    // srv := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &resolver.Resolver{DB: db}}))
    // return srv
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        writeJSON(w, http.StatusOK, map[string]string{"status": "GraphQL ready"})
    })
}
```

### 4. SSE endpoint implementation

SSE is simpler than WebSocket for one-way server→client push. Clients authenticate via the `token` query parameter (since EventSource cannot set custom headers):

```go
// backend/internal/adapters/http/rest/sse.go
package rest

import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/golang-jwt/jwt/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/infrastructure/config"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountSSE(r chi.Router, cfg *config.Config, bus events.Bus, log *logger.Logger) {
    h := &sseHandler{cfg: cfg, bus: bus, log: log}
    // SSE uses GET with token query param (EventSource cannot set headers)
    r.Get("/events", h.stream)
}

type sseHandler struct {
    cfg *config.Config
    bus events.Bus
    log *logger.Logger
}

func (h *sseHandler) stream(w http.ResponseWriter, r *http.Request) {
    // Auth via query param for EventSource compatibility
    token := r.URL.Query().Get("token")
    claims := &httpMiddleware.Claims{}
    if _, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
        return []byte(h.cfg.Auth.JWTSecret), nil
    }); err != nil {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }

    // Optional collection filter
    collection := r.URL.Query().Get("collection")

    // SSE headers
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
    w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering

    flusher, ok := w.(http.Flusher)
    if !ok {
        http.Error(w, "streaming not supported", http.StatusInternalServerError)
        return
    }

    // Send initial connected event
    fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
    flusher.Flush()

    // Subscribe to events
    eventCh := make(chan events.Event, 64)
    sub := h.bus.Subscribe(events.EventAfterCreate, func(ctx context.Context, e events.Event) error {
        if collection == "" || e.Collection == collection {
            eventCh <- e
        }
        return nil
    })
    subUpdate := h.bus.Subscribe(events.EventAfterUpdate, func(ctx context.Context, e events.Event) error {
        if collection == "" || e.Collection == collection {
            eventCh <- e
        }
        return nil
    })
    subDelete := h.bus.Subscribe(events.EventAfterDelete, func(ctx context.Context, e events.Event) error {
        if collection == "" || e.Collection == collection {
            eventCh <- e
        }
        return nil
    })
    defer h.bus.Unsubscribe(sub)
    defer h.bus.Unsubscribe(subUpdate)
    defer h.bus.Unsubscribe(subDelete)

    // Heartbeat ticker to keep connection alive through proxies
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    ctx := r.Context()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            fmt.Fprintf(w, ": heartbeat\n\n")
            flusher.Flush()
        case e := <-eventCh:
            data, err := json.Marshal(e)
            if err != nil {
                continue
            }
            fmt.Fprintf(w, "event: %s\ndata: %s\n\n", string(e.Type), data)
            flusher.Flush()
        }
    }
}
```

### 5. Frontend SSE client

```typescript
// frontend/src/lib/sse.ts
import { useAuthStore } from '@/store/auth.store'

export function subscribeToCollection(
  collection: string,
  onEvent: (event: unknown) => void,
): () => void {
  const token = useAuthStore.getState().accessToken
  const url = `/api/events?token=${token}&collection=${collection}`

  const es = new EventSource(url)

  es.addEventListener('items.after.create', (e) => onEvent(JSON.parse(e.data)))
  es.addEventListener('items.after.update', (e) => onEvent(JSON.parse(e.data)))
  es.addEventListener('items.after.delete', (e) => onEvent(JSON.parse(e.data)))

  es.onerror = () => {
    // Reconnect after 3 seconds on error
    setTimeout(() => subscribeToCollection(collection, onEvent), 3000)
  }

  // Return cleanup function
  return () => es.close()
}
```

Usage in a React component with TanStack Query:

```typescript
import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { subscribeToCollection } from '@/lib/sse'

export function useCollectionSubscription(collection: string) {
  const queryClient = useQueryClient()

  useEffect(() => {
    const unsubscribe = subscribeToCollection(collection, () => {
      // Invalidate the list query so it refetches
      queryClient.invalidateQueries({ queryKey: ['items', collection] })
    })
    return unsubscribe
  }, [collection, queryClient])
}
```

### 6. Wire SSE into the router

In `backend/internal/adapters/http/router.go`, mount SSE inside the protected group:

```go
// In the protected group:
rest.MountSSE(r, cfg, bus, log)
```

Note: SSE auth uses the query param `token` rather than the `Authorization` header — the `Authenticate` middleware cannot be used for SSE routes. Instead the SSE handler validates the token inline.

### 7. Test the REST API

```bash
TOKEN="<JWT>"
TENANT="00000000-0000-0000-0000-000000000001"

# Create a post
curl -X POST "http://localhost:8080/api/items/posts" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{"title":"Hello World","body":"<p>First post</p>","status":"published"}'

# List posts (paginated)
curl "http://localhost:8080/api/items/posts?page=1&limit=10&sort=-created_at" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"
# Response headers: X-Total-Count: 1

# Filter by status
curl "http://localhost:8080/api/items/posts?filter[status][_eq]=published" \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"

# SSE stream (open in terminal)
curl -N "http://localhost:8080/api/events?token=$TOKEN&collection=posts"
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — ItemRepository + QueryFilter interfaces
- `/home/jd/dev/full/cms/backend/internal/adapters/http/router.go` — route mounting
- `/home/jd/dev/full/cms/frontend/src/lib/sse.ts` — SSE client helper
- `/home/jd/dev/full/cms/frontend/src/lib/api.ts` — REST API client
- `/home/jd/dev/full/cms/frontend/src/lib/graphql.ts` — GraphQL client

## Next Steps

Proceed to [09 — Events & Hooks](./09-events-hooks.md) to learn about guard hooks, side-effect hooks, switching to Kafka, and writing Kafka consumers.
