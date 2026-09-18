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

type fieldRepository struct{ db *pgxpool.Pool }

const fieldSelectCols = `
		f.id, f.section_id, s.name, f.name, f.display_name, f.type, f.db_type,
		f.required, f.unique_field, f.readonly, f.hidden, f.translatable, f.quick_edit, f.show_in_table,
		f.default_value, f.validation, f.options, f.sort, f.is_system,
		f.created_at, f.updated_at, f.created_by, f.updated_by`

func (r *fieldRepository) Create(ctx context.Context, tenantID uuid.UUID, f *collectiondomain.Field) error {
	s := schemaOf(tenantID)
	defaultJSON, _ := json.Marshal(f.DefaultValue)
	validationJSON, _ := json.Marshal(f.Validation)
	optionsJSON, _ := json.Marshal(f.Options)

	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.fields
		    (id, section_id, name, display_name, type, db_type,
		     required, unique_field, readonly, hidden, translatable, quick_edit, show_in_table,
		     default_value, validation, options, sort, is_system,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, s),
		f.ID, f.SectionID, f.Name, f.DisplayName, f.Type, f.DBType,
		f.Required, f.Unique, f.Readonly, f.Hidden, f.Translatable, f.QuickEdit, f.ShowInTable,
		defaultJSON, validationJSON, optionsJSON, f.Sort, f.IsSystem,
		f.CreatedBy, f.UpdatedBy,
	)
	return err
}

func (r *fieldRepository) GetByName(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) (*collectiondomain.Field, error) {
	s := schemaOf(tenantID)
	row := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT`+fieldSelectCols+`
		FROM %s.fields f
		JOIN %s.sections s ON s.id = f.section_id
		WHERE s.collection_name=$1 AND f.name=$2 LIMIT 1`, s, s),
		collectionName, fieldName)

	f, err := scanFieldRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return f, nil
}

func (r *fieldRepository) Update(ctx context.Context, tenantID uuid.UUID, f *collectiondomain.Field) error {
	s := schemaOf(tenantID)
	defaultJSON, _ := json.Marshal(f.DefaultValue)
	validationJSON, _ := json.Marshal(f.Validation)
	optionsJSON, _ := json.Marshal(f.Options)

	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.fields
		SET display_name=$1, type=$2, db_type=$3, required=$4, unique_field=$5,
		    readonly=$6, hidden=$7, translatable=$8, quick_edit=$9, show_in_table=$10,
		    default_value=$11, validation=$12, options=$13, sort=$14,
		    section_id=$15, updated_by=$16, updated_at=NOW()
		WHERE id=$17`, s),
		f.DisplayName, f.Type, f.DBType, f.Required, f.Unique,
		f.Readonly, f.Hidden, f.Translatable, f.QuickEdit, f.ShowInTable,
		defaultJSON, validationJSON, optionsJSON, f.Sort,
		f.SectionID, f.UpdatedBy, f.ID,
	)
	return err
}

func (r *fieldRepository) Delete(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.fields f
		USING %s.sections s
		WHERE f.section_id = s.id AND s.collection_name=$1 AND f.name=$2`, s, s),
		collectionName, fieldName)
	return err
}

func (r *fieldRepository) ListByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collectiondomain.Field, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT`+fieldSelectCols+`
		FROM %s.fields f
		JOIN %s.sections s ON s.id = f.section_id
		WHERE s.collection_name=$1
		ORDER BY s.sort, s.name, f.sort, f.name`, s, s),
		collectionName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fields []*collectiondomain.Field
	for rows.Next() {
		f, err := scanFieldRow(rows)
		if err != nil {
			return nil, err
		}
		fields = append(fields, f)
	}
	return fields, rows.Err()
}

func scanFieldRow(row rowScanner) (*collectiondomain.Field, error) {
	var f collectiondomain.Field
	var defaultRaw, validationRaw, optionsRaw []byte
	err := row.Scan(
		&f.ID, &f.SectionID, &f.SectionName, &f.Name, &f.DisplayName,
		&f.Type, &f.DBType,
		&f.Required, &f.Unique, &f.Readonly, &f.Hidden, &f.Translatable, &f.QuickEdit, &f.ShowInTable,
		&defaultRaw, &validationRaw, &optionsRaw,
		&f.Sort, &f.IsSystem,
		&f.CreatedAt, &f.UpdatedAt, &f.CreatedBy, &f.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if len(defaultRaw) > 0 {
		_ = json.Unmarshal(defaultRaw, &f.DefaultValue)
	}
	if len(validationRaw) > 0 {
		_ = json.Unmarshal(validationRaw, &f.Validation)
	}
	if len(optionsRaw) > 0 {
		_ = json.Unmarshal(optionsRaw, &f.Options)
	}
	return &f, nil
}