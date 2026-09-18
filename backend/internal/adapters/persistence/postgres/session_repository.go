package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type sessionRepository struct{ db *pgxpool.Pool }

func (r *sessionRepository) Create(ctx context.Context, tenantID uuid.UUID, s *outbound.Session) error {
	sch := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sessions
		    (id, user_id, principal_collection, principal_id, refresh_token, expires_at, ip_address, user_agent)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, sch),
		s.ID, s.UserID, s.PrincipalCollection, s.PrincipalID, s.RefreshToken, s.ExpiresAt, s.IPAddress, s.UserAgent,
	)
	return err
}

func (r *sessionRepository) GetByToken(ctx context.Context, tenantID uuid.UUID, token string) (*outbound.Session, error) {
	sch := schemaOf(tenantID)
	var s outbound.Session
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, user_id, principal_collection, principal_id, refresh_token, expires_at, ip_address::text, user_agent
		FROM %s.sessions WHERE refresh_token = $1 LIMIT 1`, sch), token).
		Scan(&s.ID, &s.UserID, &s.PrincipalCollection, &s.PrincipalID, &s.RefreshToken, &s.ExpiresAt, &s.IPAddress, &s.UserAgent)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &s, nil
}

func (r *sessionRepository) Delete(ctx context.Context, tenantID uuid.UUID, token string) error {
	sch := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.sessions WHERE refresh_token = $1`, sch), token)
	return err
}

func (r *sessionRepository) DeleteAllForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	sch := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.sessions WHERE user_id=$1`, sch),
		userID)
	return err
}
