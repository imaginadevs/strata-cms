package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

// quotedIdent double-quotes a Postgres identifier to handle special characters
// (e.g. hyphens in UUID strings).
func quotedIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// TenantMigrator runs versioned SQL migrations against a per-tenant schema.
type TenantMigrator struct {
	db *pgxpool.Pool
}

// NewTenantMigrator creates a new TenantMigrator backed by the provided pool.
func NewTenantMigrator(db *pgxpool.Pool) *TenantMigrator {
	return &TenantMigrator{db: db}
}

// EnsureSchema creates the schema and runs all pending tenant migrations.
func (m *TenantMigrator) EnsureSchema(ctx context.Context, tenantID uuid.UUID) error {
	sch := quotedIdent(tenantID.String())

	// 1. Create the schema
	if _, err := m.db.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", sch)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}

	// 2. Create migrations tracking table
	if _, err := m.db.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s.schema_migrations (
			version    INT         PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`, sch)); err != nil {
		return fmt.Errorf("create migrations table: %w", err)
	}

	// 3. Apply pending migrations
	for _, mig := range tenantMigrationList {
		var applied bool
		_ = m.db.QueryRow(ctx, fmt.Sprintf(
			`SELECT EXISTS(SELECT 1 FROM %s.schema_migrations WHERE version = $1)`, sch),
			mig.version).Scan(&applied)
		if applied {
			continue
		}

		sql := strings.ReplaceAll(mig.sql, "{schema}", sch)
		if _, err := m.db.Exec(ctx, sql); err != nil {
			return fmt.Errorf("tenant migration v%d: %w", mig.version, err)
		}

		if _, err := m.db.Exec(ctx, fmt.Sprintf(
			`INSERT INTO %s.schema_migrations (version) VALUES ($1)`, sch),
			mig.version); err != nil {
			return fmt.Errorf("record tenant migration v%d: %w", mig.version, err)
		}
	}
	return nil
}

type tenantMigration struct {
	version int
	sql     string
}

// SeedDefaults inserts the default Administrator role and admin user for a
// tenant if no users exist yet. Safe to call on every startup — skips
// silently when the schema already has users.
//
// adminEmail is the login for the seeded admin (e.g. admin@<tenant-domain>),
// computed by the caller from the tenant. Default password: admin — change it
// after first login.
func (m *TenantMigrator) SeedDefaults(ctx context.Context, tenantID uuid.UUID, adminEmail string) error {
	s := quotedIdent(tenantID.String())

	// Public role always exists (idempotent). Avoid ON CONFLICT(name): some
	// tenant schemas lack a unique constraint Postgres can use for that clause.
	var publicExists bool
	if err := m.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM %s.roles WHERE name='Public')`, s),
	).Scan(&publicExists); err != nil {
		return fmt.Errorf("seed: check public role: %w", err)
	}
	if !publicExists {
		if _, err := m.db.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.roles (id, name, description, is_system)
			VALUES ($1, 'Public', 'Unauthenticated access', TRUE)`, s),
			ids.New(),
		); err != nil {
			return fmt.Errorf("seed: create public role: %w", err)
		}
	}

	// Skip the rest if users already exist.
	var count int
	if err := m.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s.users`, s)).Scan(&count); err != nil {
		return fmt.Errorf("seed: count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	// 1. Administrator role.
	var roleID uuid.UUID
	if err := m.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT id FROM %s.roles WHERE name='Administrator' LIMIT 1`, s)).
		Scan(&roleID); err != nil {
		roleID = ids.New()
		if _, err := m.db.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.roles (id, name, description, is_system)
			VALUES ($1, 'Administrator', 'Full administrative access', TRUE)`, s),
			roleID,
		); err != nil {
			return fmt.Errorf("seed: create admin role: %w", err)
		}
	}

	// 2. Admin user with hashed password.
	hash, err := bcrypt.GenerateFromPassword([]byte("admin"), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("seed: hash password: %w", err)
	}

	userID := ids.New()
	if _, err := m.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.users
		    (id, email, first_name, last_name,
		     password_hash, is_super_admin, status, provider)
		VALUES ($1, $2, 'Admin', 'User', $3, TRUE, 'active', 'local')
		ON CONFLICT (email) DO NOTHING`, s),
		userID, adminEmail, string(hash),
	); err != nil {
		return fmt.Errorf("seed: create admin user: %w", err)
	}

	// Fetch the actual user id.
	if err := m.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT id FROM %s.users WHERE email=$1 LIMIT 1`, s), adminEmail).
		Scan(&userID); err != nil {
		return fmt.Errorf("seed: fetch admin user id: %w", err)
	}

	// 3. Assign role.
	if _, err := m.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.user_roles (user_id, role_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, s),
		userID, roleID,
	); err != nil {
		return fmt.Errorf("seed: assign admin role: %w", err)
	}

	return nil
}
