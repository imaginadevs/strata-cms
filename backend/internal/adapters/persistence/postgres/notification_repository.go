package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	notifdomain "github.com/imaginacolombia/cms/internal/core/domain/notification"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type notificationRepository struct{ db *pgxpool.Pool }

func (r *notificationRepository) Create(ctx context.Context, tenantID uuid.UUID, n *notifdomain.Notification) error {
	s := schemaOf(tenantID)
	metaJSON, _ := json.Marshal(n.Meta)
	if n.Meta == nil {
		metaJSON = []byte("{}")
	}
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.notifications
		    (id, recipient_id, actor_id, type, title, body, link, meta, read_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s),
		n.ID, n.RecipientID, n.ActorID, n.Type, n.Title, n.Body, n.Link, metaJSON, n.ReadAt, n.CreatedAt,
	)
	return err
}

func (r *notificationRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*notifdomain.Notification, error) {
	s := schemaOf(tenantID)
	row := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, recipient_id, actor_id, type, title, body, link, meta, read_at, created_at
		FROM %s.notifications WHERE id=$1 LIMIT 1`, s), id)
	n, err := scanNotification(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return n, nil
}

func (r *notificationRepository) ListForUser(ctx context.Context, tenantID, userID uuid.UUID, filter outbound.NotificationFilter) ([]*notifdomain.Notification, int64, error) {
	s := schemaOf(tenantID)
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	where := "WHERE recipient_id=$1"
	args := []any{userID}
	if filter.UnreadOnly {
		where += " AND read_at IS NULL"
	}

	var total int64
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s.notifications %s`, s, where), args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, recipient_id, actor_id, type, title, body, link, meta, read_at, created_at
		FROM %s.notifications %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, s, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*notifdomain.Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, n)
	}
	return out, total, rows.Err()
}

func (r *notificationRepository) MarkRead(ctx context.Context, tenantID, userID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	tag, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.notifications SET read_at=NOW()
		WHERE id=$1 AND recipient_id=$2 AND read_at IS NULL`, s), id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		// Already read or not found / not owned — treat as success for idempotency when owned.
		var exists bool
		_ = r.db.QueryRow(ctx, fmt.Sprintf(`
			SELECT EXISTS(SELECT 1 FROM %s.notifications WHERE id=$1 AND recipient_id=$2)`, s),
			id, userID).Scan(&exists)
		if !exists {
			return pgx.ErrNoRows
		}
	}
	return nil
}

func (r *notificationRepository) MarkAllRead(ctx context.Context, tenantID, userID uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.notifications SET read_at=NOW()
		WHERE recipient_id=$1 AND read_at IS NULL`, s), userID)
	return err
}

func (r *notificationRepository) CountUnread(ctx context.Context, tenantID, userID uuid.UUID) (int64, error) {
	s := schemaOf(tenantID)
	var n int64
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s.notifications
		WHERE recipient_id=$1 AND read_at IS NULL`, s), userID).Scan(&n)
	return n, err
}

func scanNotification(row rowScanner) (*notifdomain.Notification, error) {
	var n notifdomain.Notification
	var metaRaw []byte
	err := row.Scan(
		&n.ID, &n.RecipientID, &n.ActorID, &n.Type, &n.Title, &n.Body, &n.Link,
		&metaRaw, &n.ReadAt, &n.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &n.Meta)
	}
	if n.Meta == nil {
		n.Meta = map[string]any{}
	}
	return &n, nil
}