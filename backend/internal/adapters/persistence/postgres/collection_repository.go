package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
)

type collectionRepository struct {
	db  *pgxpool.Pool
	ddl *ddlAdapter
}

const collectionCols = `
    id, name, display_name, type, icon, color, note,
    is_system, is_hidden, is_singleton, sort_field, meta, group_id, sort,
    created_at, updated_at, created_by, updated_by`

func (r *collectionRepository) Create(ctx context.Context, tenantID uuid.UUID, c *collectiondomain.Collection) error {
	s := schemaOf(tenantID)
	metaJSON, _ := json.Marshal(c.Meta)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.collections
		    (id, name, display_name, type, icon, color, note,
		     is_system, is_hidden, is_singleton, sort_field, meta, group_id, sort,
		     created_by, updated_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`, s),
		c.ID, c.Name, c.DisplayName, c.Type,
		c.Icon, c.Color, c.Note,
		c.IsSystem, c.IsHidden, c.IsSingleton, c.SortField, metaJSON, c.GroupID, c.Sort,
		c.CreatedBy, c.UpdatedBy,
	)
	return err
}

func (r *collectionRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*collectiondomain.Collection, error) {
	s := schemaOf(tenantID)
	c, err := scanOneCollection(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+collectionCols+` FROM %s.collections WHERE id=$1 LIMIT 1`, s),
		id))
	if err != nil || c == nil {
		return c, err
	}
	return c, r.loadSections(ctx, tenantID, c)
}

func (r *collectionRepository) GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*collectiondomain.Collection, error) {
	s := schemaOf(tenantID)
	c, err := scanOneCollection(r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT`+collectionCols+` FROM %s.collections WHERE name=$1 LIMIT 1`, s),
		name))
	if err != nil || c == nil {
		return c, err
	}
	return c, r.loadSections(ctx, tenantID, c)
}

func (r *collectionRepository) Update(ctx context.Context, tenantID uuid.UUID, c *collectiondomain.Collection) error {
	s := schemaOf(tenantID)
	metaJSON, _ := json.Marshal(c.Meta)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.collections
		SET display_name=$1, icon=$2, color=$3, note=$4,
		    is_hidden=$5, is_singleton=$6, sort_field=$7, meta=$8,
		    group_id=$9, sort=$10, updated_by=$11, updated_at=NOW()
		WHERE id=$12`, s),
		c.DisplayName, c.Icon, c.Color, c.Note,
		c.IsHidden, c.IsSingleton, c.SortField, metaJSON,
		c.GroupID, c.Sort, c.UpdatedBy, c.ID,
	)
	return err
}

// ReorderCollections applies group membership + sort position to many collections
// in one transaction (used by sidebar/overview drag-and-drop).
func (r *collectionRepository) ReorderCollections(ctx context.Context, tenantID uuid.UUID, items []collectiondomain.CollectionPlacement) error {
	s := schemaOf(tenantID)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, it := range items {
		if _, err := tx.Exec(ctx, fmt.Sprintf(
			`UPDATE %s.collections SET group_id=$1, sort=$2, updated_at=NOW() WHERE id=$3`, s),
			it.GroupID, it.Sort, it.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *collectionRepository) Delete(ctx context.Context, tenantID uuid.UUID, name string) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		DELETE FROM %s.fields f
		USING %s.sections sec
		WHERE f.section_id = sec.id AND sec.collection_name = $1`, s, s), name)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.sections WHERE collection_name=$1`, s), name)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.collections WHERE name=$1`, s), name)
	return err
}

func (r *collectionRepository) List(ctx context.Context, tenantID uuid.UUID, includeHidden bool) ([]*collectiondomain.Collection, error) {
	s := schemaOf(tenantID)
	q := fmt.Sprintf(`SELECT`+collectionCols+` FROM %s.collections`, s)
	if !includeHidden {
		q += " WHERE is_hidden = FALSE"
	}
	q += " ORDER BY sort, name"

	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []*collectiondomain.Collection
	for rows.Next() {
		c, err := scanCollectionRow(rows)
		if err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// ─── DDL pass-through ─────────────────────────────────────────────────────────

func (r *collectionRepository) CreateTable(ctx context.Context, tenantID uuid.UUID, c *collectiondomain.Collection) error {
	return r.ddl.CreateTable(ctx, tenantID, c)
}

func (r *collectionRepository) DropTable(ctx context.Context, tenantID uuid.UUID, name string) error {
	return r.ddl.DropTable(ctx, tenantID, name)
}

func (r *collectionRepository) AddColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collectiondomain.Field) error {
	return r.ddl.AddColumn(ctx, tenantID, collectionName, f)
}

func (r *collectionRepository) AlterColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collectiondomain.Field) error {
	return r.ddl.AlterColumn(ctx, tenantID, collectionName, f)
}

func (r *collectionRepository) DropColumn(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error {
	return r.ddl.DropColumn(ctx, tenantID, collectionName, fieldName)
}

func (r *collectionRepository) AddForeignKey(ctx context.Context, tenantID uuid.UUID, table, column, refTable string, unique, required bool, onDelete string) error {
	return r.ddl.AddForeignKey(ctx, tenantID, table, column, refTable, unique, required, onDelete)
}

func (r *collectionRepository) CreateJunctionTable(ctx context.Context, tenantID uuid.UUID, junctionTable, manyTable, oneTable string) error {
	return r.ddl.CreateJunctionTable(ctx, tenantID, junctionTable, manyTable, oneTable)
}

func (r *collectionRepository) CreateIndex(ctx context.Context, tenantID uuid.UUID, idx *collectiondomain.CollectionIndex) error {
	return r.ddl.CreateIndex(ctx, tenantID, idx)
}

func (r *collectionRepository) SetColumnUnique(ctx context.Context, tenantID uuid.UUID, tableName, columnName string, add bool) error {
	return r.ddl.SetColumnUnique(ctx, tenantID, tableName, columnName, add)
}

func (r *collectionRepository) AddCompositeUnique(ctx context.Context, tenantID uuid.UUID, tableName string, columns []string) error {
	return r.ddl.AddCompositeUnique(ctx, tenantID, tableName, columns)
}

func (r *collectionRepository) DropIndex(ctx context.Context, tenantID uuid.UUID, indexName string) error {
	return r.ddl.DropIndex(ctx, tenantID, indexName)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func (r *collectionRepository) loadSections(ctx context.Context, tenantID uuid.UUID, c *collectiondomain.Collection) error {
	s := schemaOf(tenantID)
	secRows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, collection_name, name, display_name, note, sort, visibility, translations,
		       created_at, updated_at, created_by, updated_by
		FROM %s.sections WHERE collection_name=$1 ORDER BY sort, name`, s),
		c.Name)
	if err != nil {
		return err
	}
	defer secRows.Close()

	var sections []collectiondomain.Section
	for secRows.Next() {
		sec, err := scanSectionRow(secRows)
		if err != nil {
			return err
		}
		sections = append(sections, *sec)
	}
	if err := secRows.Err(); err != nil {
		return err
	}

	fieldRows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT`+fieldSelectCols+`
		FROM %s.fields f
		JOIN %s.sections s ON s.id = f.section_id
		WHERE s.collection_name=$1
		ORDER BY s.sort, s.name, f.sort, f.name`, s, s),
		c.Name)
	if err != nil {
		return err
	}
	defer fieldRows.Close()

	fieldsBySection := map[uuid.UUID][]collectiondomain.Field{}
	for fieldRows.Next() {
		f, err := scanFieldRow(fieldRows)
		if err != nil {
			return err
		}
		fieldsBySection[f.SectionID] = append(fieldsBySection[f.SectionID], *f)
	}
	if err := fieldRows.Err(); err != nil {
		return err
	}

	for i := range sections {
		sections[i].Fields = fieldsBySection[sections[i].ID]
		if sections[i].Fields == nil {
			sections[i].Fields = []collectiondomain.Field{}
		}
	}
	c.Sections = sections
	return nil
}

func scanOneCollection(row pgx.Row) (*collectiondomain.Collection, error) {
	c, err := scanCollectionRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return c, nil
}

func scanCollectionRow(row rowScanner) (*collectiondomain.Collection, error) {
	var c collectiondomain.Collection
	var metaRaw []byte
	err := row.Scan(
		&c.ID, &c.Name, &c.DisplayName, &c.Type,
		&c.Icon, &c.Color, &c.Note,
		&c.IsSystem, &c.IsHidden, &c.IsSingleton, &c.SortField, &metaRaw, &c.GroupID, &c.Sort,
		&c.CreatedAt, &c.UpdatedAt, &c.CreatedBy, &c.UpdatedBy,
	)
	if err != nil {
		return nil, err
	}
	if len(metaRaw) > 0 {
		_ = json.Unmarshal(metaRaw, &c.Meta)
	}
	return &c, nil
}

func (r *collectionRepository) RenameCollection(ctx context.Context, tenantID uuid.UUID, oldName, newName string) error {
	s := schemaOf(tenantID)
	oldT := safeIdent(oldName)
	newT := safeIdent(newName)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, fmt.Sprintf(`ALTER TABLE IF EXISTS %s.%s RENAME TO %s`, s, oldT, newT))
	if err != nil {
		return fmt.Errorf("rename table: %w", err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.collections SET name=$2, updated_at=NOW() WHERE name=$1`, s), oldName, newName)
	if err != nil {
		return fmt.Errorf("rename collections row: %w", err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.sections SET collection_name=$2 WHERE collection_name=$1`, s), oldName, newName)
	if err != nil {
		return fmt.Errorf("rename sections: %w", err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.relations SET collection_many=$2 WHERE collection_many=$1`, s), oldName, newName)
	if err != nil {
		return fmt.Errorf("rename relations many: %w", err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.relations SET collection_one=$2 WHERE collection_one=$1`, s), oldName, newName)
	if err != nil {
		return fmt.Errorf("rename relations one: %w", err)
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`UPDATE %s.collection_indexes SET collection_name=$2 WHERE collection_name=$1`, s), oldName, newName)
	if err != nil && !strings.Contains(err.Error(), "does not exist") {
		return fmt.Errorf("rename indexes: %w", err)
	}
	return tx.Commit(ctx)
}
