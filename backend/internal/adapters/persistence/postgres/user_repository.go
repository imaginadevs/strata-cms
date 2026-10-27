package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type userRepository struct{ db *pgxpool.Pool }

const userCols = `
    id, email, first_name, last_name, avatar,
    status, provider, provider_uid, password_hash, last_login,
    language, ui_language, is_super_admin, created_at, updated_at, created_by, updated_by`

func (r *userRepository) Create(ctx context.Context, tenantID uuid.UUID, u *userdomain.User) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.users
		    (id, email, first_name, last_name, avatar,
		     status, provider, provider_uid, password_hash, language, ui_language,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, s),
		u.ID, u.Email, u.FirstName, u.LastName, u.Avatar,
		u.Status, u.Provider, u.ProviderUID, u.PasswordHash, u.Language, u.UILanguage,
		u.CreatedBy, u.UpdatedBy,
	)
	return err
}

func (r *userRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*userdomain.User, error) {
	s := schemaOf(tenantID)
	u, err := scanOneUser(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+userCols+` FROM %s.users WHERE id=$1 LIMIT 1`, s),
		id))
	if err != nil {
		return nil, err
	}
	if u != nil {
		if err := r.loadRoles(ctx, tenantID, u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (r *userRepository) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*userdomain.User, error) {
	s := schemaOf(tenantID)
	u, err := scanOneUser(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+userCols+` FROM %s.users WHERE email=$1 LIMIT 1`, s),
		email))
	if err != nil {
		return nil, err
	}
	if u != nil {
		if err := r.loadRoles(ctx, tenantID, u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (r *userRepository) GetByProviderUID(ctx context.Context, tenantID uuid.UUID, provider userdomain.AuthProvider, uid string) (*userdomain.User, error) {
	s := schemaOf(tenantID)
	return scanOneUser(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+userCols+` FROM %s.users WHERE provider=$1 AND provider_uid=$2 LIMIT 1`, s),
		provider, uid))
}

func (r *userRepository) Update(ctx context.Context, tenantID uuid.UUID, u *userdomain.User) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users
		SET email=$1, first_name=$2, last_name=$3, avatar=$4,
		    status=$5, provider=$6, provider_uid=$7, language=$8, ui_language=$9,
		    updated_by=$10, updated_at=NOW()
		WHERE id=$11`, s),
		u.Email, u.FirstName, u.LastName, u.Avatar,
		u.Status, u.Provider, u.ProviderUID, u.Language, u.UILanguage,
		u.UpdatedBy, u.ID,
	)
	return err
}

func (r *userRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.users WHERE id=$1`, s),
		id)
	return err
}

func (r *userRepository) List(ctx context.Context, tenantID uuid.UUID, filter outbound.UserFilter) ([]*userdomain.User, int64, error) {
	s := schemaOf(tenantID)
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	var args []any
	var conditions []string

	if filter.Status != nil {
		args = append(args, *filter.Status)
		conditions = append(conditions, fmt.Sprintf("status = $%d", len(args)))
	}
	if filter.Search != "" {
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		conditions = append(conditions, fmt.Sprintf(
			"(LOWER(email) LIKE $%d OR LOWER(first_name) LIKE $%d OR LOWER(last_name) LIKE $%d)",
			len(args), len(args), len(args),
		))
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	countRow := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s.users`, s)+where, args...)
	var total int64
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "created_at DESC"
	if filter.Sort != "" {
		orderBy = filter.Sort
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx,
		fmt.Sprintf(`SELECT`+userCols+` FROM %s.users`, s)+where+
			fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, orderBy, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []*userdomain.User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (r *userRepository) SetPassword(ctx context.Context, tenantID, userID uuid.UUID, hash string) error {
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`UPDATE %s.users SET password_hash=$1, updated_at=NOW() WHERE id=$2`, schemaOf(tenantID)),
		hash, userID)
	return err
}

func (r *userRepository) RecordLogin(ctx context.Context, tenantID, userID uuid.UUID) error {
	now := time.Now().UTC()
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`UPDATE %s.users SET last_login=$1, updated_at=NOW() WHERE id=$2`, schemaOf(tenantID)),
		now, userID)
	return err
}

func (r *userRepository) SetInvite(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, fmt.Sprintf(
		`UPDATE %s.users SET invite_token_hash=$1, invite_expires_at=$2, updated_at=NOW() WHERE id=$3`,
		schemaOf(tenantID)), tokenHash, expiresAt, userID)
	return err
}

func (r *userRepository) GetByInviteToken(ctx context.Context, tenantID uuid.UUID, tokenHash string) (*userdomain.User, error) {
	s := schemaOf(tenantID)
	// Only return the user while the invite is still valid (unexpired).
	u, err := scanUserRow(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+userCols+` FROM %s.users
			WHERE invite_token_hash=$1 AND invite_expires_at > NOW() LIMIT 1`, s), tokenHash))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func (r *userRepository) AcceptInvite(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) error {
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.users
		SET password_hash=$1, status='active', invite_token_hash=NULL, invite_expires_at=NULL, updated_at=NOW()
		WHERE id=$2`, schemaOf(tenantID)), passwordHash, userID)
	return err
}

func (r *userRepository) ListAdminEmails(ctx context.Context, tenantID uuid.UUID) ([]string, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT u.email
		FROM %s.users u
		LEFT JOIN %s.user_roles ur ON ur.user_id = u.id
		LEFT JOIN %s.roles r ON r.id = ur.role_id
		WHERE u.status = 'active'
		  AND (u.is_super_admin = TRUE OR r.name = 'Administrator')`, s, s, s))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var emails []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		if e != "" {
			emails = append(emails, e)
		}
	}
	return emails, rows.Err()
}

// ─── role loading ─────────────────────────────────────────────────────────────

func (r *userRepository) loadRoles(ctx context.Context, tenantID uuid.UUID, u *userdomain.User) error {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT r.id, r.name, r.description, r.icon, r.is_system,
		       r.created_at, r.updated_at, r.created_by, r.updated_by
		FROM %s.roles r
		JOIN %s.user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1`, s, s),
		u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var role userdomain.Role
		err := rows.Scan(
			&role.ID, &role.Name, &role.Description,
			&role.Icon, &role.IsSystem,
			&role.CreatedAt, &role.UpdatedAt, &role.CreatedBy, &role.UpdatedBy,
		)
		if err != nil {
			return err
		}
		u.Roles = append(u.Roles, role)
	}
	return rows.Err()
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

func scanOneUser(row pgx.Row) (*userdomain.User, error) {
	u, err := scanUserRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return u, nil
}

func scanUserRow(row rowScanner) (*userdomain.User, error) {
	var u userdomain.User
	err := row.Scan(
		&u.ID, &u.Email, &u.FirstName, &u.LastName, &u.Avatar,
		&u.Status, &u.Provider, &u.ProviderUID, &u.PasswordHash, &u.LastLogin,
		&u.Language, &u.UILanguage, &u.IsSuperAdmin, &u.CreatedAt, &u.UpdatedAt, &u.CreatedBy, &u.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
