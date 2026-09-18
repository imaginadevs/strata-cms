package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

type relationRepository struct{ db *pgxpool.Pool }

func (r *relationRepository) Create(ctx context.Context, tenantID uuid.UUID, rel *collectiondomain.Relation) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.relations
		    (id, collection_many, field_many, collection_one, field_one,
		     type, junction_table, on_delete, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s),
		rel.ID, rel.CollectionMany, rel.FieldMany,
		rel.CollectionOne, rel.FieldOne,
		rel.Type, rel.JunctionTable, rel.OnDelete,
		rel.CreatedBy,
	)
	return err
}

func (r *relationRepository) List(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collectiondomain.Relation, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, collection_many, field_many, collection_one, field_one,
		       type, junction_table, on_delete, created_at, updated_at, created_by
		FROM %s.relations
		WHERE collection_many=$1 OR collection_one=$1
		ORDER BY created_at`, s),
		collectionName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rels []*collectiondomain.Relation
	for rows.Next() {
		var rel collectiondomain.Relation
		err := rows.Scan(
			&rel.ID,
			&rel.CollectionMany, &rel.FieldMany,
			&rel.CollectionOne, &rel.FieldOne,
			&rel.Type, &rel.JunctionTable, &rel.OnDelete,
			&rel.CreatedAt, &rel.UpdatedAt, &rel.CreatedBy,
		)
		if err != nil {
			return nil, err
		}
		rels = append(rels, &rel)
	}
	return rels, rows.Err()
}

func (r *relationRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx,
		fmt.Sprintf(`DELETE FROM %s.relations WHERE id=$1`, s),
		id)
	return err
}
