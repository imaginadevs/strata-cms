package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
)

type roleRepository struct{ db *pgxpool.Pool }

const roleCols = `
    id, name, description, icon, is_system,
    created_at, updated_at, created_by, updated_by`

func (r *roleRepository) Create(ctx context.Context, tenantID uuid.UUID, role *userdomain.Role) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.roles
		    (id, name, description, icon, is_system, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, s),
		role.ID, role.Name, role.Description,
		role.Icon, role.IsSystem, role.CreatedBy, role.UpdatedBy,
	)
	return err
}

func (r *roleRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*userdomain.Role, error) {
	s := schemaOf(tenantID)
	return scanOneRole(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+roleCols+` FROM %s.roles WHERE id=$1 LIMIT 1`, s),
		id))
}

func (r *roleRepository) GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*userdomain.Role, error) {
	s := schemaOf(tenantID)
	return scanOneRole(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+roleCols+` FROM %s.roles WHERE name=$1 LIMIT 1`, s),
		name))
}

func (r *roleRepository) Update(ctx context.Context, tenantID uuid.UUID, role *userdomain.Role) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.roles
		SET name=$1, description=$2, icon=$3, updated_by=$4, updated_at=NOW()
		WHERE id=$5`, s),
		role.Name, role.Description, role.Icon, role.UpdatedBy, role.ID,
	)
	return err
}

func (r *roleRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.roles WHERE id=$1`, s),
		id)
	return err
}

func (r *roleRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*userdomain.Role, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx,
		fmt.Sprintf(`SELECT`+roleCols+` FROM %s.roles ORDER BY name`, s))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*userdomain.Role
	for rows.Next() {
		role, err := scanRoleRow(rows)
		if err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (r *roleRepository) AssignToUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.user_roles (user_id, role_id)
		VALUES ($1,$2) ON CONFLICT DO NOTHING`, s),
		userID, roleID,
	)
	return err
}

func (r *roleRepository) RevokeFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.user_roles WHERE user_id=$1 AND role_id=$2`, s),
		userID, roleID,
	)
	return err
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

func scanOneRole(row pgx.Row) (*userdomain.Role, error) {
	role, err := scanRoleRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return role, nil
}

func scanRoleRow(row rowScanner) (*userdomain.Role, error) {
	var role userdomain.Role
	err := row.Scan(
		&role.ID, &role.Name, &role.Description,
		&role.Icon, &role.IsSystem,
		&role.CreatedAt, &role.UpdatedAt, &role.CreatedBy, &role.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &role, nil
}
