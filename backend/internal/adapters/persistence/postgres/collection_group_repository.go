package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

type collectionGroupRepository struct{ db *pgxpool.Pool }

const collectionGroupCols = `id, name, icon, color, sort, created_at, updated_at, created_by, updated_by`

func scanGroupRow(row rowScanner) (*collectiondomain.CollectionGroup, error) {
	var g collectiondomain.CollectionGroup
	err := row.Scan(&g.ID, &g.Name, &g.Icon, &g.Color, &g.Sort,
		&g.CreatedAt, &g.UpdatedAt, &g.CreatedBy, &g.UpdatedBy)
	if err != nil {
		return nil, err
	}
	return &g, nil
}

func (r *collectionGroupRepository) List(ctx context.Context, tenantID uuid.UUID) ([]*collectiondomain.CollectionGroup, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(
		`SELECT `+collectionGroupCols+` FROM %s.collection_groups ORDER BY sort, name`, s))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*collectiondomain.CollectionGroup
	for rows.Next() {
		g, err := scanGroupRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *collectionGroupRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*collectiondomain.CollectionGroup, error) {
	s := schemaOf(tenantID)
	g, err := scanGroupRow(r.db.QueryRow(ctx, fmt.Sprintf(
		`SELECT `+collectionGroupCols+` FROM %s.collection_groups WHERE id=$1`, s), id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *collectionGroupRepository) Create(ctx context.Context, tenantID uuid.UUID, g *collectiondomain.CollectionGroup) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.collection_groups (id, name, icon, color, sort, created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, s),
		g.ID, g.Name, g.Icon, g.Color, g.Sort, g.CreatedBy, g.UpdatedBy)
	return err
}

func (r *collectionGroupRepository) Update(ctx context.Context, tenantID uuid.UUID, g *collectiondomain.CollectionGroup) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.collection_groups
		SET name=$1, icon=$2, color=$3, sort=$4, updated_by=$5, updated_at=NOW()
		WHERE id=$6`, s),
		g.Name, g.Icon, g.Color, g.Sort, g.UpdatedBy, g.ID)
	return err
}

func (r *collectionGroupRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	// collections.group_id has ON DELETE SET NULL, so member collections become ungrouped.
	_, err := r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.collection_groups WHERE id=$1`, s), id)
	return err
}

func (r *collectionGroupRepository) Reorder(ctx context.Context, tenantID uuid.UUID, orderedIDs []uuid.UUID) error {
	s := schemaOf(tenantID)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for i, id := range orderedIDs {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s.collection_groups SET sort=$1, updated_at=NOW() WHERE id=$2`, s), i, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
