package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

type sectionRepository struct{ db *pgxpool.Pool }

func (r *sectionRepository) Create(ctx context.Context, tenantID uuid.UUID, s *collectiondomain.Section) error {
	sch := schemaOf(tenantID)
	visJSON, err := marshalVisibility(s.Visibility)
	if err != nil {
		return err
	}
	trJSON, _ := json.Marshal(s.Translations)
	if s.Translations == nil {
		trJSON = []byte("[]")
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.sections
		    (id, collection_name, name, display_name, note, sort, visibility, translations,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, sch),
		s.ID, s.CollectionName, s.Name, s.DisplayName, s.Note, s.Sort,
		visJSON, trJSON, s.CreatedBy, s.UpdatedBy,
	)
	return err
}

func (r *sectionRepository) GetByName(ctx context.Context, tenantID uuid.UUID, collectionName, sectionName string) (*collectiondomain.Section, error) {
	sch := schemaOf(tenantID)
	row := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, collection_name, name, display_name, note, sort, visibility, translations,
		       created_at, updated_at, created_by, updated_by
		FROM %s.sections WHERE collection_name=$1 AND name=$2 LIMIT 1`, sch),
		collectionName, sectionName)
	s, err := scanSectionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return s, nil
}

func (r *sectionRepository) Update(ctx context.Context, tenantID uuid.UUID, s *collectiondomain.Section) error {
	sch := schemaOf(tenantID)
	visJSON, err := marshalVisibility(s.Visibility)
	if err != nil {
		return err
	}
	trJSON, _ := json.Marshal(s.Translations)
	if s.Translations == nil {
		trJSON = []byte("[]")
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.sections
		SET display_name=$1, note=$2, sort=$3, visibility=$4, translations=$5,
		    updated_by=$6, updated_at=NOW()
		WHERE collection_name=$7 AND name=$8`, sch),
		s.DisplayName, s.Note, s.Sort, visJSON, trJSON,
		s.UpdatedBy, s.CollectionName, s.Name,
	)
	return err
}

func (r *sectionRepository) Delete(ctx context.Context, tenantID uuid.UUID, collectionName, sectionName string) error {
	sch := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.sections WHERE collection_name=$1 AND name=$2`, sch),
		collectionName, sectionName)
	return err
}

func (r *sectionRepository) ListByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collectiondomain.Section, error) {
	sch := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, collection_name, name, display_name, note, sort, visibility, translations,
		       created_at, updated_at, created_by, updated_by
		FROM %s.sections WHERE collection_name=$1 ORDER BY sort, name`, sch),
		collectionName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*collectiondomain.Section
	for rows.Next() {
		s, err := scanSectionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sectionRepository) CountByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) (int, error) {
	sch := schemaOf(tenantID)
	var n int
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s.sections WHERE collection_name=$1`, sch),
		collectionName).Scan(&n)
	return n, err
}

func (r *sectionRepository) ReassignFields(ctx context.Context, tenantID, fromSectionID, toSectionID uuid.UUID) error {
	sch := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.fields SET section_id=$1, updated_at=NOW() WHERE section_id=$2`, sch),
		toSectionID, fromSectionID)
	return err
}

func marshalVisibility(c *collectiondomain.Condition) ([]byte, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

func scanSectionRow(row rowScanner) (*collectiondomain.Section, error) {
	var s collectiondomain.Section
	var visRaw, trRaw []byte
	err := row.Scan(
		&s.ID, &s.CollectionName, &s.Name, &s.DisplayName, &s.Note, &s.Sort,
		&visRaw, &trRaw,
		&s.CreatedAt, &s.UpdatedAt, &s.CreatedBy, &s.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if len(visRaw) > 0 && string(visRaw) != "null" {
		var c collectiondomain.Condition
		if err := json.Unmarshal(visRaw, &c); err != nil {
			return nil, fmt.Errorf("unmarshal visibility: %w", err)
		}
		s.Visibility = &c
	}
	if len(trRaw) > 0 {
		_ = json.Unmarshal(trRaw, &s.Translations)
	}
	if s.Translations == nil {
		s.Translations = []collectiondomain.Translation{}
	}
	return &s, nil
}