# 07 — Permissions

## Overview

Access control uses a role-based model where each role is assigned per-collection permissions covering four actions (create, read, update, delete). Permissions also carry a field-level allow-list and a JSONB row-level filter expression. System roles (Administrator, Editor, Viewer) are seeded by the initial migration and cannot be deleted. This guide covers the permission data model, evaluation logic in the item repository, and the handlers for managing roles and permissions.

## Prerequisites

- Guides 05 and 06 completed (tenants, collections, auth system working).
- The seed data in migration `001` has been applied (system roles exist).

## Step-by-Step

### 1. Permission data model recap

From `backend/internal/core/domain/user/entity.go`:

```go
type Permission struct {
    domain.Base

    RoleID     uuid.UUID `json:"role_id"`
    Collection string    `json:"collection"` // "*" = all collections
    Action     Action    `json:"action"`     // create | read | update | delete
    Fields     []string  `json:"fields"`     // "*" = all fields; empty = no field restriction
    Filter     any       `json:"filter"`     // JSONB row filter, e.g. {"status":{"_eq":"published"}}
}

type Action string
const (
    ActionCreate Action = "create"
    ActionRead   Action = "read"
    ActionUpdate Action = "update"
    ActionDelete Action = "delete"
)
```

The `permissions` table has a UNIQUE constraint on `(tenant_id, role_id, collection, action)`, so there is at most one permission record per role-collection-action triple. To update a permission, use upsert.

### 2. System roles seeded by migration 001

```sql
INSERT INTO roles (id, tenant_id, name, description, is_system) VALUES
  ('00000000-0000-0000-0000-000000000010', '00000000-0000-0000-0000-000000000001',
   'Administrator', 'Full access', TRUE),
  ('00000000-0000-0000-0000-000000000011', '00000000-0000-0000-0000-000000000001',
   'Editor', 'Can create and edit content', TRUE),
  ('00000000-0000-0000-0000-000000000012', '00000000-0000-0000-0000-000000000001',
   'Viewer', 'Read-only access', TRUE);
```

System roles (`is_system = TRUE`) can have their permissions changed but cannot be deleted. When creating a new tenant, these roles should be seeded as part of the tenant creation flow.

### 3. Permission repository implementation

Create `backend/internal/adapters/postgres/permission_repository.go`:

```go
package postgres

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/lib/pq"

    "github.com/imaginacolombia/cms/internal/core/domain/user"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type PermissionRepository struct {
    db *pgxpool.Pool
}

func NewPermissionRepository(db *pgxpool.Pool) *PermissionRepository {
    return &PermissionRepository{db: db}
}

// Upsert inserts or updates a permission record.
// The unique key is (tenant_id, role_id, collection, action).
func (r *PermissionRepository) Upsert(ctx context.Context, p *user.Permission) error {
    fieldsJSON, _ := json.Marshal(p.Fields)
    filterJSON, _ := json.Marshal(p.Filter)

    _, err := r.db.Exec(ctx, `
        INSERT INTO permissions (id, tenant_id, role_id, collection, action, fields, filter, created_at, updated_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, NOW(), NOW())
        ON CONFLICT (tenant_id, role_id, collection, action)
        DO UPDATE SET fields = EXCLUDED.fields, filter = EXCLUDED.filter, updated_at = NOW()`,
        p.ID, p.TenantID, p.RoleID, p.Collection, p.Action, fieldsJSON, filterJSON,
    )
    return err
}

func (r *PermissionRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM permissions WHERE tenant_id = $1 AND id = $2`,
        tenantID, id,
    )
    return err
}

func (r *PermissionRepository) GetForRole(
    ctx context.Context,
    tenantID, roleID uuid.UUID,
) ([]*user.Permission, error) {
    rows, err := r.db.Query(ctx, `
        SELECT id, tenant_id, role_id, collection, action, fields, filter, created_at, updated_at
        FROM   permissions
        WHERE  tenant_id = $1 AND role_id = $2`,
        tenantID, roleID,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var perms []*user.Permission
    for rows.Next() {
        p, err := scanPermission(rows)
        if err != nil {
            return nil, err
        }
        perms = append(perms, p)
    }
    return perms, rows.Err()
}

// GetForUser resolves the effective permission for a user by joining
// through user_roles → permissions. Returns the most permissive match
// (any role granting access is sufficient — OR semantics).
func (r *PermissionRepository) GetForUser(
    ctx context.Context,
    tenantID, userID uuid.UUID,
    collectionName string,
    action user.Action,
) (*user.Permission, error) {
    const q = `
        SELECT p.id, p.tenant_id, p.role_id, p.collection, p.action, p.fields, p.filter,
               p.created_at, p.updated_at
        FROM   permissions p
        JOIN   user_roles ur ON ur.role_id = p.role_id AND ur.tenant_id = p.tenant_id
        WHERE  ur.user_id = $1
          AND  p.tenant_id = $2
          AND  p.collection IN ($3, '*')
          AND  p.action = $4
        ORDER  BY p.collection DESC   -- specific collection before wildcard "*"
        LIMIT  1`

    return scanPermission(r.db.QueryRow(ctx, q, userID, tenantID, collectionName, action))
}

func scanPermission(row interface{ Scan(dest ...any) error }) (*user.Permission, error) {
    p := &user.Permission{}
    var fieldsJSON, filterJSON []byte
    err := row.Scan(
        &p.ID, &p.TenantID, &p.RoleID, &p.Collection, &p.Action,
        &fieldsJSON, &filterJSON,
        &p.CreatedAt, &p.UpdatedAt,
    )
    if err != nil {
        return nil, err
    }
    _ = json.Unmarshal(fieldsJSON, &p.Fields)
    _ = json.Unmarshal(filterJSON, &p.Filter)
    return p, nil
}
```

### 4. Permission evaluation in the item repository

When `ItemRepository.List()` is called, it must enforce permissions before running the query. Create a permission evaluator:

```go
// backend/internal/adapters/postgres/permission_evaluator.go
package postgres

import (
    "context"
    "fmt"

    "github.com/google/uuid"

    "github.com/imaginacolombia/cms/internal/core/domain/user"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// PermissionEvaluator checks whether a user can perform an action
// and applies field restrictions and row filters.
type PermissionEvaluator struct {
    permRepo outbound.PermissionRepository
}

func NewPermissionEvaluator(permRepo outbound.PermissionRepository) *PermissionEvaluator {
    return &PermissionEvaluator{permRepo: permRepo}
}

type EvalResult struct {
    Allowed      bool
    AllowedFields []string  // nil = all fields permitted
    RowFilter    map[string]any
}

func (e *PermissionEvaluator) Evaluate(
    ctx context.Context,
    tenantID, userID uuid.UUID,
    collection string,
    action user.Action,
) (*EvalResult, error) {
    perm, err := e.permRepo.GetForUser(ctx, tenantID, userID, collection, action)
    if err != nil {
        // No permission record found — deny
        return &EvalResult{Allowed: false}, nil
    }

    result := &EvalResult{Allowed: true}

    // Field restrictions
    if len(perm.Fields) > 0 && !(len(perm.Fields) == 1 && perm.Fields[0] == "*") {
        result.AllowedFields = perm.Fields
    }

    // Row-level filter
    if perm.Filter != nil {
        if m, ok := perm.Filter.(map[string]any); ok && len(m) > 0 {
            result.RowFilter = m
        }
    }

    return result, nil
}
```

Apply in the item list query:

```go
// Inside ItemRepository.List():
eval, err := r.evaluator.Evaluate(ctx, tenantID, userID, collectionName, user.ActionRead)
if err != nil || !eval.Allowed {
    return nil, 0, fmt.Errorf("permission denied")
}

// Apply field restriction
selectedFields := "*"
if len(eval.AllowedFields) > 0 {
    quoted := make([]string, len(eval.AllowedFields))
    for i, f := range eval.AllowedFields {
        quoted[i] = pgIdentifier(f)
    }
    selectedFields = strings.Join(quoted, ", ")
}

// Apply row-level filter (translate JSONB filter to SQL)
extraWhere, extraArgs := buildRowFilter(eval.RowFilter, existingArgCount)
```

### 5. Row-level filter expression format

Filters use a Directus-inspired JSONB expression format:

```json
{"status": {"_eq": "published"}}
{"created_by": {"_eq": "$CURRENT_USER"}}
{"_or": [
    {"status": {"_eq": "published"}},
    {"created_by": {"_eq": "$CURRENT_USER"}}
]}
```

Translate to SQL in the adapter:

```go
// buildRowFilter converts a filter expression to a SQL WHERE fragment.
// Returns empty string and nil args if filter is nil.
func buildRowFilter(filter map[string]any, argOffset int) (string, []any) {
    if len(filter) == 0 {
        return "", nil
    }
    clauses := []string{}
    args := []any{}
    i := argOffset + 1

    for field, condition := range filter {
        if field == "_or" || field == "_and" {
            // Recursively handle logical operators — left as an exercise
            continue
        }
        if condMap, ok := condition.(map[string]any); ok {
            for op, val := range condMap {
                sqlOp := filterOpToSQL(op)
                if sqlOp == "" {
                    continue
                }
                // $CURRENT_USER is replaced at runtime with the actual user UUID
                clauses = append(clauses, fmt.Sprintf(`"%s" %s $%d`, field, sqlOp, i))
                args = append(args, val)
                i++
            }
        }
    }

    if len(clauses) == 0 {
        return "", nil
    }
    return " AND (" + strings.Join(clauses, " AND ") + ")", args
}

func filterOpToSQL(op string) string {
    switch op {
    case "_eq":   return "="
    case "_neq":  return "<>"
    case "_lt":   return "<"
    case "_lte":  return "<="
    case "_gt":   return ">"
    case "_gte":  return ">="
    case "_like": return "ILIKE"
    case "_in":   return "= ANY"
    default:      return ""
    }
}
```

### 6. Assigning roles to users

```go
// In RoleRepository.AssignToUser():
func (r *RoleRepository) AssignToUser(
    ctx context.Context,
    tenantID, userID, roleID uuid.UUID,
) error {
    _, err := r.db.Exec(ctx, `
        INSERT INTO user_roles (user_id, role_id, tenant_id)
        VALUES ($1, $2, $3)
        ON CONFLICT DO NOTHING`,
        userID, roleID, tenantID,
    )
    return err
}

func (r *RoleRepository) RevokeFromUser(
    ctx context.Context,
    tenantID, userID, roleID uuid.UUID,
) error {
    _, err := r.db.Exec(ctx, `
        DELETE FROM user_roles
        WHERE user_id = $1 AND role_id = $2 AND tenant_id = $3`,
        userID, roleID, tenantID,
    )
    return err
}
```

HTTP handlers:

```go
// POST /api/users/{id}/roles
func (h *userHandler) assignRole(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
    userID, _ := uuid.Parse(chi.URLParam(r, "id"))

    var body struct {
        RoleID uuid.UUID `json:"role_id"`
    }
    json.NewDecoder(r.Body).Decode(&body)

    if err := h.roleRepo.AssignToUser(r.Context(), tenantID, userID, body.RoleID); err != nil {
        writeError(w, http.StatusInternalServerError, "could not assign role")
        return
    }
    w.WriteHeader(http.StatusNoContent)
}
```

### 7. Permission management API

```bash
# List permissions for the Editor role
curl http://localhost:8080/api/roles/$EDITOR_ROLE_ID/permissions \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT"

# Grant Editor role read access to "posts", all fields, only published items
curl -X POST http://localhost:8080/api/roles/$EDITOR_ROLE_ID/permissions \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "posts",
    "action": "read",
    "fields": ["*"],
    "filter": {"status": {"_eq": "published"}}
  }'

# Grant Editor role create access to "posts", restricted to specific fields
curl -X POST http://localhost:8080/api/roles/$EDITOR_ROLE_ID/permissions \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{
    "collection": "posts",
    "action": "create",
    "fields": ["title", "body", "status"],
    "filter": {}
  }'

# Assign Editor role to a user
curl -X POST http://localhost:8080/api/users/$USER_ID/roles \
  -H "Authorization: Bearer $TOKEN" \
  -H "X-Tenant-ID: $TENANT" \
  -H "Content-Type: application/json" \
  -d '{"role_id": "'$EDITOR_ROLE_ID'"}'
```

### 8. Administrator role — wildcard permissions

The Administrator role should have full access. Seed it via API or migration:

```sql
-- Grant Administrator full CRUD on all collections
INSERT INTO permissions (id, tenant_id, role_id, collection, action, fields, filter)
SELECT
    uuid_generate_v4(),
    '00000000-0000-0000-0000-000000000001',
    '00000000-0000-0000-0000-000000000010',   -- Administrator role
    '*',
    action,
    '["*"]',
    '{}'
FROM unnest(ARRAY['create','read','update','delete']) AS action
ON CONFLICT DO NOTHING;
```

### 9. Checking permissions in middleware

For a request-level permission check before reaching the handler:

```go
// RequirePermission returns a middleware that aborts with 403
// if the authenticated user lacks the given permission.
func RequirePermission(
    permRepo outbound.PermissionRepository,
    collectionName string,
    action user.Action,
) func(http.Handler) http.Handler {
    evaluator := NewPermissionEvaluator(permRepo)

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
            userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
            if !ok {
                http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
                return
            }

            result, err := evaluator.Evaluate(r.Context(), tenantID, userID, collectionName, action)
            if err != nil || !result.Allowed {
                http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
                return
            }

            next.ServeHTTP(w, r)
        })
    }
}
```

Usage in a route group:

```go
r.With(RequirePermission(permRepo, "posts", user.ActionCreate)).Post("/items/posts", h.createItem)
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/core/domain/user/entity.go` — Role, Permission, Action types
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — RoleRepository + PermissionRepository interfaces
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/001_initial_schema.sql` — permissions, roles, user_roles tables + system role seed

## Next Steps

Proceed to [08 — API Layer](./08-api-layer.md) to implement REST item endpoints, pagination, GraphQL schema generation, and the SSE event stream.
