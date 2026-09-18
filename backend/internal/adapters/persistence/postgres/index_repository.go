package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

type indexRepository struct {
	db *pgxpool.Pool
}

func (r *indexRepository) Create(ctx context.Context, tenantID uuid.UUID, idx *collectiondomain.CollectionIndex) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.collection_indexes
		    (id, collection_name, name, type, fields, created_by)
		VALUES ($1, $2, $3, $4, $5, $6)`, s),
		idx.ID, idx.CollectionName, idx.Name, idx.Type, idx.Fields, idx.CreatedBy,
	)
	return err
}

func (r *indexRepository) List(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collectiondomain.CollectionIndex, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, collection_name, name, type, fields, created_at, created_by
		FROM %s.collection_indexes
		WHERE collection_name = $1
		ORDER BY created_at`, s),
		collectionName,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*collectiondomain.CollectionIndex
	for rows.Next() {
		var idx collectiondomain.CollectionIndex
		if err := rows.Scan(
			&idx.ID, &idx.CollectionName, &idx.Name,
			&idx.Type, &idx.Fields, &idx.CreatedAt, &idx.CreatedBy,
		); err != nil {
			return nil, err
		}
		result = append(result, &idx)
	}
	return result, rows.Err()
}

func (r *indexRepository) Delete(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.collection_indexes WHERE id = $1`, s),
		id,
	)
	return err
}

func (r *indexRepository) GetByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (*collectiondomain.CollectionIndex, error) {
	s := schemaOf(tenantID)
	var idx collectiondomain.CollectionIndex
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, collection_name, name, type, fields, created_at, created_by
		FROM %s.collection_indexes
		WHERE id = $1`, s),
		id,
	).Scan(&idx.ID, &idx.CollectionName, &idx.Name,
		&idx.Type, &idx.Fields, &idx.CreatedAt, &idx.CreatedBy)
	if err != nil {
		return nil, err
	}
	return &idx, nil
}
