package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	tenantdomain "github.com/imaginacolombia/cms/internal/core/domain/tenant"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type tenantRepository struct{ db *pgxpool.Pool }

const tenantCols = `
    id, name, slug, default_language, logo, primary_color, domain,
    active, is_platform, settings, storage_used_bytes, storage_quota_bytes,
    created_at, updated_at`

func (r *tenantRepository) Create(ctx context.Context, t *tenantdomain.Tenant) error {
	b, _ := json.Marshal(t.Settings)

	_, err := r.db.Exec(ctx, `
		INSERT INTO tenants
		    (id, name, slug, default_language, logo, primary_color, domain, active, is_platform, settings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		t.ID, t.Name, t.Slug, t.DefaultLanguage,
		t.Logo, t.PrimaryColor, t.Domain, t.Active, t.IsPlatform, b,
	)
	if err != nil {
		return fmt.Errorf("tenant create: insert: %w", err)
	}
	// Per-tenant schema provisioning is handled by TenantMigrator.EnsureSchema,
	// which is called during bootstrap and should be called by the tenant creation
	// handler after this function returns.
	return nil
}

func (r *tenantRepository) GetByID(ctx context.Context, id uuid.UUID) (*tenantdomain.Tenant, error) {
	return scanOneTenant(r.db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE id = $1 LIMIT 1`, id))
}

func (r *tenantRepository) GetBySlug(ctx context.Context, slug string) (*tenantdomain.Tenant, error) {
	return scanOneTenant(r.db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE slug = $1 LIMIT 1`, slug))
}

func (r *tenantRepository) GetByDomain(ctx context.Context, domain string) (*tenantdomain.Tenant, error) {
	return scanOneTenant(r.db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE domain = $1 LIMIT 1`, domain))
}

// GetPlatform returns the single platform tenant, or nil if none is flagged.
func (r *tenantRepository) GetPlatform(ctx context.Context) (*tenantdomain.Tenant, error) {
	return scanOneTenant(r.db.QueryRow(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE is_platform = TRUE LIMIT 1`))
}

// SetPlatform makes id the one-and-only platform tenant (clears any previous
// flag first, in a transaction, so the single-platform unique index is upheld).
func (r *tenantRepository) SetPlatform(ctx context.Context, id uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx,
		`UPDATE tenants SET is_platform = FALSE WHERE is_platform = TRUE AND id <> $1`, id); err != nil {
		return fmt.Errorf("clear platform flag: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE tenants SET is_platform = TRUE WHERE id = $1`, id); err != nil {
		return fmt.Errorf("set platform flag: %w", err)
	}
	return tx.Commit(ctx)
}

func (r *tenantRepository) Update(ctx context.Context, t *tenantdomain.Tenant) error {
	b, _ := json.Marshal(t.Settings)
	_, err := r.db.Exec(ctx, `
		UPDATE tenants
		SET name=$1, slug=$2, default_language=$3, logo=$4, primary_color=$5,
		    domain=$6, active=$7, settings=$8, updated_at=NOW()
		WHERE id=$9`,
		t.Name, t.Slug, t.DefaultLanguage, t.Logo, t.PrimaryColor,
		t.Domain, t.Active, b, t.ID,
	)
	return err
}

func (r *tenantRepository) AddStorageUsed(ctx context.Context, id uuid.UUID, delta int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tenants
		SET storage_used_bytes = GREATEST(0, storage_used_bytes + $1)
		WHERE id = $2`,
		delta, id,
	)
	return err
}

func (r *tenantRepository) List(ctx context.Context, filter outbound.TenantFilter) ([]*tenantdomain.Tenant, int64, error) {
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	where := "TRUE"
	args := []any{}
	if filter.Active != nil {
		args = append(args, *filter.Active)
		where = fmt.Sprintf("active = $%d", len(args))
	}

	countRow := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM tenants WHERE `+where, args...)
	var total int64
	if err := countRow.Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx,
		`SELECT`+tenantCols+` FROM tenants WHERE `+where+
			fmt.Sprintf(` ORDER BY name LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tenants []*tenantdomain.Tenant
	for rows.Next() {
		t, err := scanTenantRow(rows)
		if err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}
	return tenants, total, rows.Err()
}

func (r *tenantRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.Exec(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	return err
}

// ─── scan helpers ─────────────────────────────────────────────────────────────

func scanOneTenant(row pgx.Row) (*tenantdomain.Tenant, error) {
	t, err := scanTenantRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanTenantRow(row rowScanner) (*tenantdomain.Tenant, error) {
	var t tenantdomain.Tenant
	var settingsRaw []byte
	err := row.Scan(
		&t.ID, &t.Name, &t.Slug, &t.DefaultLanguage,
		&t.Logo, &t.PrimaryColor, &t.Domain, &t.Active, &t.IsPlatform,
		&settingsRaw,
		&t.StorageUsedBytes, &t.StorageQuotaBytes,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &t.Settings)
	}
	t.Settings.NormalizePlugins()
	return &t, nil
}

func normalisePage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	return page, limit
}
