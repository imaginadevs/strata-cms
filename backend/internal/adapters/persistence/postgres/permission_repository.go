package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
)

type permissionRepository struct{ db *pgxpool.Pool }

func (r *permissionRepository) Upsert(ctx context.Context, tenantID uuid.UUID, p *userdomain.Permission) error {
	s := schemaOf(tenantID)
	filterJSON, _ := json.Marshal(p.Filter)

	// `fields` is a Postgres TEXT[] — pass the slice natively so pgx encodes a
	// proper array literal. A nil slice becomes an empty array to satisfy the
	// NOT NULL column (json-encoding it would write "null" → malformed array).
	fields := p.Fields
	if fields == nil {
		fields = []string{}
	}

	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.permissions
		    (id, role_id, collection, action, fields, filter,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (role_id, collection, action) DO UPDATE
		SET fields=EXCLUDED.fields, filter=EXCLUDED.filter,
		    updated_by=EXCLUDED.updated_by, updated_at=NOW()`, s),
		p.ID, p.RoleID, p.Collection, p.Action,
		fields, filterJSON, p.CreatedBy, p.UpdatedBy,
	)
	return err
}

func (r *permissionRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.permissions WHERE id=$1`, s),
		id)
	return err
}

func (r *permissionRepository) GetForRole(ctx context.Context, tenantID, roleID uuid.UUID) ([]*userdomain.Permission, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, role_id, collection, action, fields, filter,
		       created_at, updated_at, created_by, updated_by
		FROM %s.permissions WHERE role_id=$1
		ORDER BY collection, action`, s),
		roleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*userdomain.Permission
	for rows.Next() {
		p, err := scanPermRow(rows)
		if err != nil {
			return nil, err
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

func (r *permissionRepository) GetForUser(ctx context.Context, tenantID, userID uuid.UUID, coll string, action userdomain.Action) (*userdomain.Permission, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT p.id, p.role_id, p.collection, p.action, p.fields, p.filter,
		       p.created_at, p.updated_at, p.created_by, p.updated_by
		FROM %s.permissions p
		JOIN %s.user_roles ur ON ur.role_id = p.role_id
		WHERE ur.user_id=$1
		  AND (p.collection=$2 OR p.collection='*')
		  AND (p.action=$3 OR p.action='all' OR p.action='*')
		LIMIT 1`, s, s),
		userID, coll, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		return scanPermRow(rows)
	}
	return nil, rows.Err()
}

func (r *permissionRepository) GetForRoleNames(ctx context.Context, tenantID uuid.UUID, roleNames []string, coll string, action userdomain.Action) (*userdomain.Permission, error) {
	if len(roleNames) == 0 {
		return nil, nil
	}
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT p.id, p.role_id, p.collection, p.action, p.fields, p.filter,
		       p.created_at, p.updated_at, p.created_by, p.updated_by
		FROM %s.permissions p
		JOIN %s.roles r ON r.id = p.role_id
		WHERE r.name = ANY($1)
		  AND (p.collection=$2 OR p.collection='*')
		  AND (p.action=$3 OR p.action='all' OR p.action='*')
		LIMIT 1`, s, s),
		roleNames, coll, action)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		return scanPermRow(rows)
	}
	return nil, rows.Err()
}

func (r *permissionRepository) DeleteForRole(ctx context.Context, tenantID, roleID uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.permissions WHERE role_id=$1`, s),
		roleID)
	return err
}

func (r *permissionRepository) ReplaceForRole(ctx context.Context, tenantID, roleID uuid.UUID, perms []*userdomain.Permission) error {
	if err := r.DeleteForRole(ctx, tenantID, roleID); err != nil {
		return err
	}
	for _, p := range perms {
		p.RoleID = roleID
		if err := r.Upsert(ctx, tenantID, p); err != nil {
			return err
		}
	}
	return nil
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

func scanPermRow(row rowScanner) (*userdomain.Permission, error) {
	var p userdomain.Permission
	var filterRaw []byte
	err := row.Scan(
		&p.ID, &p.RoleID, &p.Collection, &p.Action,
		&p.Fields, &filterRaw, // fields is TEXT[] → scanned natively into []string
		&p.CreatedAt, &p.UpdatedAt, &p.CreatedBy, &p.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if len(filterRaw) > 0 {
		_ = json.Unmarshal(filterRaw, &p.Filter)
	}
	return &p, nil
}
