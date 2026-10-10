package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
)

type apiTokenRepository struct{ db *pgxpool.Pool }

const apiTokenCols = `
    id, name, secret_prefix, secret_hash, old_secret_hash, old_secret_expires_at,
    policies, lifetime, rotate_at, last_notified_at, revoked, last_used_at,
    created_at, updated_at, created_by, updated_by`

func scanAPIToken(row rowScanner) (*apitoken.APIToken, error) {
	var t apitoken.APIToken
	var policiesRaw []byte
	err := row.Scan(
		&t.ID, &t.Name, &t.SecretPrefix, &t.SecretHash, &t.OldSecretHash, &t.OldSecretExpiresAt,
		&policiesRaw, &t.Lifetime, &t.RotateAt, &t.LastNotifiedAt, &t.Revoked, &t.LastUsedAt,
		&t.CreatedAt, &t.UpdatedAt, &t.CreatedBy, &t.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if len(policiesRaw) > 0 {
		_ = json.Unmarshal(policiesRaw, &t.Policies)
	}
	if t.Policies == nil {
		t.Policies = []apitoken.Policy{}
	}
	return &t, nil
}

func (r *apiTokenRepository) Create(ctx context.Context, tenantID uuid.UUID, t *apitoken.APIToken) error {
	s := schemaOf(tenantID)
	policies, _ := json.Marshal(t.Policies)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.api_tokens
		    (id, name, secret_prefix, secret_hash, old_secret_hash, old_secret_expires_at,
		     policies, lifetime, rotate_at, last_notified_at, revoked, last_used_at,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, s),
		t.ID, t.Name, t.SecretPrefix, t.SecretHash, t.OldSecretHash, t.OldSecretExpiresAt,
		policies, t.Lifetime, t.RotateAt, t.LastNotifiedAt, t.Revoked, t.LastUsedAt,
		t.CreatedBy, t.UpdatedBy,
	)
	return err
}

func (r *apiTokenRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*apitoken.APIToken, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`SELECT`+apiTokenCols+` FROM %s.api_tokens ORDER BY created_at DESC`, s))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*apitoken.APIToken
	for rows.Next() {
		t, err := scanAPIToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (r *apiTokenRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*apitoken.APIToken, error) {
	return r.getBy(ctx, tenantID, "id", id)
}

func (r *apiTokenRepository) GetByPrefix(ctx context.Context, tenantID uuid.UUID, prefix string) (*apitoken.APIToken, error) {
	return r.getBy(ctx, tenantID, "secret_prefix", prefix)
}

func (r *apiTokenRepository) getBy(ctx context.Context, tenantID uuid.UUID, col string, val any) (*apitoken.APIToken, error) {
	s := schemaOf(tenantID)
	t, err := scanAPIToken(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+apiTokenCols+` FROM %s.api_tokens WHERE %s=$1 LIMIT 1`, s, col), val))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return t, nil
}

func (r *apiTokenRepository) Update(ctx context.Context, tenantID uuid.UUID, t *apitoken.APIToken) error {
	s := schemaOf(tenantID)
	policies, _ := json.Marshal(t.Policies)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.api_tokens
		SET name=$1, secret_hash=$2, old_secret_hash=$3, old_secret_expires_at=$4,
		    policies=$5, lifetime=$6, rotate_at=$7, last_notified_at=$8, revoked=$9,
		    updated_by=$10, updated_at=NOW()
		WHERE id=$11`, s),
		t.Name, t.SecretHash, t.OldSecretHash, t.OldSecretExpiresAt,
		policies, t.Lifetime, t.RotateAt, t.LastNotifiedAt, t.Revoked,
		t.UpdatedBy, t.ID,
	)
	return err
}

func (r *apiTokenRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.api_tokens WHERE id=$1`, s), id)
	return err
}

func (r *apiTokenRepository) TouchLastUsed(ctx context.Context, tenantID, id uuid.UUID, at time.Time) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`UPDATE %s.api_tokens SET last_used_at=$1 WHERE id=$2`, s), at, id)
	return err
}
