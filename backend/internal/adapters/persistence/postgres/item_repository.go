package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// itemRepository performs CRUD on dynamically-named tenant collection tables.
// Every table lives in the per-tenant schema (cms_{hex8}).
// Table and column names are validated through safeIdent before use in SQL.
type itemRepository struct{ db *pgxpool.Pool }

func (r *itemRepository) Create(ctx context.Context, tenantID uuid.UUID, collectionName string, data map[string]any) (map[string]any, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	qualName := schema + "." + table

	data["id"] = ids.New().String()

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

	sql := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s) RETURNING *",
		qualName,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
	rows, err := r.db.Query(ctx, sql, values...)
	if err != nil {
		return nil, fmt.Errorf("item create: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		return pgxRowsToMap(rows)
	}
	return data, nil
}

// GetByID fetches a single row by id. filter.Filter, when non-empty, adds
// extra equality conditions (ANDed with id=$1) — used to enforce row-level
// permission filters (e.g. "customer_id = $CURRENT_USER") so a caller who
// isn't the owner gets a not-found result instead of another tenant user's row.
func (r *itemRepository) GetByID(ctx context.Context, tenantID uuid.UUID, collectionName, id string, filter outbound.QueryFilter) (map[string]any, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)

	conds := []string{"id=$1"}
	args := []any{id}
	i := 2
	for k, v := range filter.Filter {
		conds = append(conds, fmt.Sprintf("%s::text=$%d", safeIdent(k), i))
		args = append(args, fmt.Sprint(v))
		i++
	}
	if filter.Lang != "" {
		conds = append(conds, fmt.Sprintf("language=$%d", i))
		args = append(args, filter.Lang)
		i++
	}

	rows, err := r.db.Query(ctx,
		fmt.Sprintf("SELECT * FROM %s.%s WHERE %s LIMIT 1", schema, table, strings.Join(conds, " AND ")),
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		return pgxRowsToMap(rows)
	}
	return nil, nil
}

func (r *itemRepository) List(ctx context.Context, tenantID uuid.UUID, collectionName string, filter outbound.QueryFilter) ([]map[string]any, int64, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	qualName := schema + "." + table

	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	// Build the WHERE clause: tenant scope + optional equality filters + language.
	// Equality compares on ::text so a plain string value works against any column
	// type (uuid, int, …). Field names are validated via safeIdent. Values are
	// always bind parameters — never interpolated.
	var conds []string
	args := []any{}
	i := 1
	for k, v := range filter.Filter {
		conds = append(conds, fmt.Sprintf("%s::text=$%d", safeIdent(k), i))
		args = append(args, fmt.Sprint(v))
		i++
	}
	if filter.Lang != "" {
		conds = append(conds, fmt.Sprintf("language=$%d", i))
		args = append(args, filter.Lang)
		i++
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	var total int64
	if err := r.db.QueryRow(ctx,
		fmt.Sprintf("SELECT COUNT(*) FROM %s%s", qualName, where), args...).
		Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "created_at DESC"
	if len(filter.Sort) > 0 {
		parts := make([]string, 0, len(filter.Sort))
		for _, s := range filter.Sort {
			dir := "ASC"
			if s.Desc {
				dir = "DESC"
			}
			parts = append(parts, safeIdent(s.Field)+" "+dir)
		}
		orderBy = strings.Join(parts, ", ")
	}

	sql := fmt.Sprintf(
		"SELECT * FROM %s%s ORDER BY %s LIMIT $%d OFFSET $%d",
		qualName, where, orderBy, i, i+1,
	)
	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var items []map[string]any
	for rows.Next() {
		item, err := pgxRowsToMap(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (r *itemRepository) Update(ctx context.Context, tenantID uuid.UUID, collectionName, id string, data map[string]any) (map[string]any, error) {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	qualName := schema + "." + table

	delete(data, "id")
	delete(data, "tenant_id")
	delete(data, "created_at")
	delete(data, "created_by")

	sets := make([]string, 0, len(data))
	values := make([]any, 0, len(data)+2)
	i := 1
	for k, v := range data {
		sets = append(sets, fmt.Sprintf("%s=$%d", safeIdent(k), i))
		values = append(values, v)
		i++
	}
	sets = append(sets, "updated_at=NOW()")
	values = append(values, id)

	sql := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id=$%d RETURNING *",
		qualName, strings.Join(sets, ", "), i,
	)
	rows, err := r.db.Query(ctx, sql, values...)
	if err != nil {
		return nil, fmt.Errorf("item update: %w", err)
	}
	defer rows.Close()

	if rows.Next() {
		return pgxRowsToMap(rows)
	}
	return nil, nil
}

func (r *itemRepository) Delete(ctx context.Context, tenantID uuid.UUID, collectionName, id string) error {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)

	_, err := r.db.Exec(ctx,
		fmt.Sprintf("DELETE FROM %s.%s WHERE id=$1", schema, table),
		id)
	return err
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// pgxRowsToMap converts the current row of a pgx.Rows result into a plain map.
// Column names come from FieldDescriptions(); values come from Values().
// This is needed for dynamic-schema tables that have no fixed struct.
func pgxRowsToMap(rows pgx.Rows) (map[string]any, error) {
	vals, err := rows.Values()
	if err != nil {
		return nil, err
	}
	descs := rows.FieldDescriptions()
	m := make(map[string]any, len(descs))
	for i, fd := range descs {
		m[string(fd.Name)] = normalisePGValue(vals[i])
	}
	return m, nil
}

// normalisePGValue turns pgx UUID wire forms into plain strings so JSON
// clients never see a 16-byte array (which String()'s to "57,67,…").
func normalisePGValue(v any) any {
	switch t := v.(type) {
	case [16]byte:
		return uuid.UUID(t).String()
	case pgtype.UUID:
		if !t.Valid {
			return nil
		}
		return uuid.UUID(t.Bytes).String()
	default:
		return v
	}
}
