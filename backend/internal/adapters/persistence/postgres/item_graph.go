package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx, so the row helpers below
// can run either directly against the pool or inside a transaction.
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// idString normalises a value that may hold a UUID in any of the forms pgx can
// decode it into, so parent/child ids compare reliably regardless of codec.
func idString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case [16]byte:
		return uuid.UUID(t).String()
	case pgtype.UUID:
		if !t.Valid {
			return ""
		}
		return uuid.UUID(t.Bytes).String()
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}

// ChildrenByFK loads child rows whose fkColumn is in parentIDs. The FK is compared
// on ::text so a []string of parent ids works against a uuid column. Ordered by
// the supplied sort fields, defaulting to created_at.
func (r *itemRepository) ChildrenByFK(ctx context.Context, tenantID uuid.UUID, collectionName, fkColumn string, parentIDs []string, sort []outbound.SortField) ([]map[string]any, error) {
	if len(parentIDs) == 0 {
		return nil, nil
	}
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	fk := safeIdent(fkColumn)

	orderBy := "created_at ASC"
	if len(sort) > 0 {
		parts := make([]string, 0, len(sort))
		for _, s := range sort {
			dir := "ASC"
			if s.Desc {
				dir = "DESC"
			}
			parts = append(parts, safeIdent(s.Field)+" "+dir)
		}
		orderBy = strings.Join(parts, ", ")
	}

	sql := fmt.Sprintf(
		"SELECT * FROM %s.%s WHERE %s::text = ANY($1) ORDER BY %s",
		schema, table, fk, orderBy,
	)
	rows, err := r.db.Query(ctx, sql, parentIDs)
	if err != nil {
		return nil, fmt.Errorf("children by fk: %w", err)
	}
	defer rows.Close()

	var out []map[string]any
	for rows.Next() {
		m, err := pgxRowsToMap(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveGraph upserts the parent then reconciles each child relation, all in one tx.
func (r *itemRepository) SaveGraph(ctx context.Context, tenantID uuid.UUID, collectionName, id string, parent map[string]any, children []outbound.ChildReconcile, isCreate bool) (map[string]any, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op once committed

	var parentRow map[string]any
	var parentID string

	if isCreate {
		parentRow, parentID, err = insertRow(ctx, tx, tenantID, collectionName, parent)
		if err != nil {
			return nil, err
		}
	} else {
		parentRow, err = updateRow(ctx, tx, tenantID, collectionName, id, parent)
		if err != nil {
			return nil, err
		}
		if parentRow == nil {
			return nil, nil // not found — nothing committed
		}
		parentID = id
	}

	for _, ch := range children {
		if err := reconcileChildren(ctx, tx, tenantID, ch, parentID); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return parentRow, nil
}

// insertRow inserts one row, assigning a fresh id and the tenant id, and returns
// the stored row plus the generated id.
func insertRow(ctx context.Context, q querier, tenantID uuid.UUID, collectionName string, data map[string]any) (map[string]any, string, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)

	id := ids.New().String()
	data["id"] = id

	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	values := make([]any, 0, len(data))
	i := 1
	for k, v := range data {
		cols = append(cols, safeIdent(k))
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, v)
		i++
	}

	sql := fmt.Sprintf("INSERT INTO %s.%s (%s) VALUES (%s) RETURNING *",
		schema, table, strings.Join(cols, ", "), strings.Join(placeholders, ", "))
	rows, err := q.Query(ctx, sql, values...)
	if err != nil {
		return nil, "", fmt.Errorf("insert row: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		m, err := pgxRowsToMap(rows)
		return m, id, err
	}
	return data, id, rows.Err()
}

// updateRow updates one row by id within the tenant, returning the stored row or
// nil if no row matched.
func updateRow(ctx context.Context, q querier, tenantID uuid.UUID, collectionName, id string, data map[string]any) (map[string]any, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)

	delete(data, "id")
	delete(data, "tenant_id")
	delete(data, "created_at")
	delete(data, "created_by")

	sets := make([]string, 0, len(data)+1)
	values := make([]any, 0, len(data)+2)
	i := 1
	for k, v := range data {
		sets = append(sets, fmt.Sprintf("%s=$%d", safeIdent(k), i))
		values = append(values, v)
		i++
	}
	sets = append(sets, "updated_at=NOW()")
	values = append(values, id)

	sql := fmt.Sprintf("UPDATE %s.%s SET %s WHERE id=$%d RETURNING *",
		schema, table, strings.Join(sets, ", "), i)
	rows, err := q.Query(ctx, sql, values...)
	if err != nil {
		return nil, fmt.Errorf("update row: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return pgxRowsToMap(rows)
	}
	return nil, rows.Err()
}

// reconcileChildren applies the desired child set for one relation: rows with an
// id are updated, rows without are inserted, and — when the relation is owned —
// existing rows omitted from the desired set are deleted. The FK column is forced
// to the parent id on every incoming row so a client cannot reparent a child.
func reconcileChildren(ctx context.Context, q querier, tenantID uuid.UUID, ch outbound.ChildReconcile, parentID string) error {
	schema := schemaOf(tenantID)
	table := safeIdent(ch.Collection)
	fk := safeIdent(ch.FKColumn)

	// Existing child ids for this parent.
	existing := map[string]bool{}
	rows, err := q.Query(ctx,
		fmt.Sprintf("SELECT id::text FROM %s.%s WHERE %s::text=$1", schema, table, fk),
		parentID)
	if err != nil {
		return fmt.Errorf("reconcile: load existing: %w", err)
	}
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			rows.Close()
			return err
		}
		existing[s] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	incoming := map[string]bool{}
	for _, row := range ch.Rows {
		row[ch.FKColumn] = parentID // enforce parentage
		idStr := idString(row["id"])
		if idStr != "" {
			// Only rows already parented here may be updated — prevents a client
			// from reparenting/stealing another parent's child within the tenant.
			if !existing[idStr] {
				return fmt.Errorf("reconcile: child %s does not belong to this parent", idStr)
			}
			incoming[idStr] = true
			if _, err := updateRow(ctx, q, tenantID, ch.Collection, idStr, row); err != nil {
				return fmt.Errorf("reconcile: update child: %w", err)
			}
		} else {
			if _, _, err := insertRow(ctx, q, tenantID, ch.Collection, row); err != nil {
				return fmt.Errorf("reconcile: insert child: %w", err)
			}
		}
	}

	if ch.Owned {
		for cid := range existing {
			if incoming[cid] {
				continue
			}
			if _, err := q.Exec(ctx,
				fmt.Sprintf("DELETE FROM %s.%s WHERE id=$1", schema, table),
				cid); err != nil {
				return fmt.Errorf("reconcile: delete child: %w", err)
			}
		}
	}
	return nil
}
