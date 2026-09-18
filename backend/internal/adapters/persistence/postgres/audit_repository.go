package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type auditRepository struct{ db *pgxpool.Pool }

func (r *auditRepository) Create(ctx context.Context, tenantID uuid.UUID, log *auditdomain.Log) error {
	s := schemaOf(tenantID)
	oldJSON, _ := json.Marshal(log.OldData)
	newJSON, _ := json.Marshal(log.NewData)

	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.audit_logs
		    (id, user_id, user_email, collection, item_id,
		     operation, old_data, new_data, changed_fields,
		     ip_address, user_agent, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, s),
		log.ID, log.UserID, log.UserEmail,
		log.Collection, log.ItemID,
		log.Operation, oldJSON, newJSON, log.ChangedFields,
		log.IPAddress, log.UserAgent, log.CreatedAt,
	)
	return err
}

func (r *auditRepository) List(ctx context.Context, tenantID uuid.UUID, filter outbound.AuditFilter) ([]*auditdomain.Log, int64, error) {
	s := schemaOf(tenantID)
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	var args []any
	var conditions []string
	if filter.Collection != nil {
		args = append(args, *filter.Collection)
		conditions = append(conditions, fmt.Sprintf("collection = $%d", len(args)))
	}
	if filter.UserID != nil {
		args = append(args, *filter.UserID)
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if filter.Operation != nil {
		args = append(args, *filter.Operation)
		conditions = append(conditions, fmt.Sprintf("operation = $%d", len(args)))
	}

	where := ""
	for i, c := range conditions {
		if i == 0 {
			where = " WHERE " + c
		} else {
			where += " AND " + c
		}
	}

	var total int64
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s.audit_logs`, s)+where, args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx,
		fmt.Sprintf(`SELECT id, user_id, user_email, collection, item_id,
		        operation, old_data, new_data, changed_fields,
		        ip_address::text, user_agent, created_at
		 FROM %s.audit_logs`, s)+where+
			fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d OFFSET $%d`, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var logs []*auditdomain.Log
	for rows.Next() {
		var l auditdomain.Log
		var oldRaw, newRaw []byte
		err := rows.Scan(
			&l.ID, &l.UserID, &l.UserEmail,
			&l.Collection, &l.ItemID,
			&l.Operation, &oldRaw, &newRaw, &l.ChangedFields,
			&l.IPAddress, &l.UserAgent, &l.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if len(oldRaw) > 0 {
			_ = json.Unmarshal(oldRaw, &l.OldData)
		}
		if len(newRaw) > 0 {
			_ = json.Unmarshal(newRaw, &l.NewData)
		}
		logs = append(logs, &l)
	}
	return logs, total, rows.Err()
}
