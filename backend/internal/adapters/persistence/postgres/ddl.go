package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/domain/collection"
)

// executor abstracts pgxpool.Pool so DDL can run inside a transaction or
// directly against the pool without changing the call site.
type executor interface {
	Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error)
}

// pgxPoolExecutor adapts *pgxpool.Pool to the executor interface.
type pgxPoolExecutor struct{ db *pgxpool.Pool }

func (p *pgxPoolExecutor) Exec(ctx context.Context, sql string, args ...any) (interface{ RowsAffected() int64 }, error) {
	return p.db.Exec(ctx, sql, args...)
}

// ddlAdapter performs schema-management DDL on per-tenant Postgres schemas.
// All table names are fully qualified (schema.table) so they work with
// PgBouncer in transaction-pool mode (no search_path dependency).
type ddlAdapter struct {
	db *pgxpool.Pool
}

func newDDLAdapter(db *pgxpool.Pool) *ddlAdapter {
	return &ddlAdapter{db: db}
}

// CreateSchema creates the per-tenant schema if it does not already exist.
func (d *ddlAdapter) CreateSchema(ctx context.Context, tenantID uuid.UUID) error {
	schema := schemaOf(tenantID)
	_, err := d.db.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema))
	return err
}

// CreateTable creates a new collection table in the tenant's schema.
// The table always includes the base system columns (id, tenant_id, timestamps,
// actor IDs) before any user-defined fields.
func (d *ddlAdapter) CreateTable(ctx context.Context, tenantID uuid.UUID, c *collection.Collection) error {
	schema := schemaOf(tenantID)
	table := safeIdent(c.Name)
	qualName := schema + "." + table

	cols := []string{
		"id          UUID        NOT NULL DEFAULT uuid_generate_v4() PRIMARY KEY",
		// language stores which locale this item's content is written in.
		// It behaves like id — always present, not shown in the field editor.
		"language    VARCHAR(10) NOT NULL DEFAULT 'en'",
		"created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"created_by  UUID",
		"updated_by  UUID",
	}
	for _, f := range c.AllFields() {
		if f.IsSystem {
			continue // system fields have their own column definitions below
		}
		cols = append(cols, fieldColumnDef(f))
	}

	sql := fmt.Sprintf("CREATE TABLE %s (\n  %s\n)", qualName, strings.Join(cols, ",\n  "))
	_, err := d.db.Exec(ctx, sql)
	return err
}

// DropTable drops a collection table from the tenant's schema.
func (d *ddlAdapter) DropTable(ctx context.Context, tenantID uuid.UUID, tableName string) error {
	schema := schemaOf(tenantID)
	table := safeIdent(tableName)
	_, err := d.db.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s.%s", schema, table))
	return err
}

// AddColumn adds a new column to an existing collection table.
func (d *ddlAdapter) AddColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collection.Field) error {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	col := safeIdent(f.Name)
	colType := pgType(*f)

	sql := fmt.Sprintf("ALTER TABLE %s.%s ADD COLUMN %s %s", schema, table, col, colType)
	if f.Required && f.DefaultValue == nil {
		sql += " NOT NULL"
	}
	_, err := d.db.Exec(ctx, sql)
	return err
}

// AlterColumn changes the type of an existing column.
// Renaming is handled separately (DropColumn + AddColumn) to keep this simple.
func (d *ddlAdapter) AlterColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collection.Field) error {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	col := safeIdent(f.Name)
	colType := pgType(*f)

	sql := fmt.Sprintf(
		"ALTER TABLE %s.%s ALTER COLUMN %s TYPE %s USING %s::%s",
		schema, table, col, colType, col, colType,
	)
	_, err := d.db.Exec(ctx, sql)
	return err
}

// DropColumn removes a column from a collection table.
func (d *ddlAdapter) DropColumn(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error {
	schema := schemaOf(tenantID)
	table := safeIdent(collectionName)
	col := safeIdent(fieldName)

	_, err := d.db.Exec(ctx,
		fmt.Sprintf("ALTER TABLE %s.%s DROP COLUMN IF EXISTS %s", schema, table, col))
	return err
}

// globalTables lists top-level system tables that live in the public schema
// rather than in a per-tenant cms_{hex8} schema. When these appear as a
// relation target the FK must reference "public".table instead.
var globalTables = map[string]bool{
	"users": true,
}

// refSchema returns the fully-qualified schema identifier to use when
// referencing refTable in a foreign key constraint.
// tenantSchemaName is the raw (unquoted) schema name — e.g. a UUID string.
func refSchema(tenantSchemaName, refTable string) string {
	if globalTables[refTable] {
		return safeIdent("public")
	}
	return quotedIdent(tenantSchemaName)
}

// AddForeignKey adds a UUID column with an inline FOREIGN KEY constraint to an
// existing collection table. onDelete must be "cascade", "set_null", or "restrict".
// required=true makes the column NOT NULL; in that case onDelete must not be "set_null".
func (d *ddlAdapter) AddForeignKey(ctx context.Context, tenantID uuid.UUID, table, column, refTable string, unique, required bool, onDelete string) error {
	ts := tenantSchema(tenantID)
	schema := quotedIdent(ts)
	col := safeIdent(column)
	tbl := safeIdent(table)
	ref := safeIdent(refTable)

	switch onDelete {
	case "cascade":
		onDelete = "CASCADE"
	case "restrict":
		onDelete = "RESTRICT"
	default:
		onDelete = "SET NULL"
	}

	modifiers := ""
	if required {
		modifiers += " NOT NULL"
	}
	if unique {
		modifiers += " UNIQUE"
	}

	sql := fmt.Sprintf(
		`ALTER TABLE %s.%s ADD COLUMN %s UUID%s REFERENCES %s.%s(id) ON DELETE %s`,
		schema, tbl, col, modifiers, refSchema(ts, refTable), ref, onDelete,
	)
	_, err := d.db.Exec(ctx, sql)
	return err
}

// CreateJunctionTable creates a many-to-many junction table with two FK columns
// and a composite primary key.
func (d *ddlAdapter) CreateJunctionTable(ctx context.Context, tenantID uuid.UUID, junctionTable, manyTable, oneTable string) error {
	ts := tenantSchema(tenantID)
	schema := quotedIdent(ts)
	jt := safeIdent(junctionTable)
	many := safeIdent(manyTable)
	one := safeIdent(oneTable)
	manyCol := safeIdent(manyTable + "_id")
	oneCol := safeIdent(oneTable + "_id")

	sql := fmt.Sprintf(`
		CREATE TABLE %s.%s (
			%s UUID NOT NULL REFERENCES %s.%s(id) ON DELETE CASCADE,
			%s UUID NOT NULL REFERENCES %s.%s(id) ON DELETE CASCADE,
			PRIMARY KEY (%s, %s)
		)`,
		schema, jt,
		manyCol, refSchema(ts, manyTable), many,
		oneCol, refSchema(ts, oneTable), one,
		manyCol, oneCol,
	)
	_, err := d.db.Exec(ctx, sql)
	return err
}

// CreateIndex creates a B-tree or full-text GIN index on a collection table.
func (d *ddlAdapter) CreateIndex(ctx context.Context, tenantID uuid.UUID, idx *collection.CollectionIndex) error {
	ts := tenantSchema(tenantID)
	schema := quotedIdent(ts)
	table := safeIdent(idx.CollectionName)
	idxName := safeIdent(idx.Name)

	switch idx.Type {
	case collection.IndexTypeBTree:
		cols := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			cols[i] = safeIdent(f)
		}
		sql := fmt.Sprintf("CREATE INDEX %s ON %s.%s (%s)",
			idxName, schema, table, strings.Join(cols, ", "))
		_, err := d.db.Exec(ctx, sql)
		return err

	case collection.IndexTypeFullText:
		parts := make([]string, len(idx.Fields))
		for i, f := range idx.Fields {
			parts[i] = fmt.Sprintf("COALESCE(%s::text, '')", safeIdent(f))
		}
		tsExpr := fmt.Sprintf("to_tsvector('simple', %s)", strings.Join(parts, " || ' ' || "))
		sqlStr := fmt.Sprintf("CREATE INDEX %s ON %s.%s USING GIN (%s)",
			idxName, schema, table, tsExpr)
		_, err := d.db.Exec(ctx, sqlStr)
		return err

	default:
		return fmt.Errorf("unknown index type: %s", idx.Type)
	}
}

// DropIndex removes an index from the tenant schema.
func (d *ddlAdapter) DropIndex(ctx context.Context, tenantID uuid.UUID, indexName string) error {
	schema := schemaOf(tenantID)
	_, err := d.db.Exec(ctx,
		fmt.Sprintf("DROP INDEX IF EXISTS %s.%s", schema, safeIdent(indexName)))
	return err
}

// AddCompositeUnique adds a multi-column UNIQUE constraint to a collection table.
// Idempotent: silently succeeds if the constraint already exists (Postgres error 42710).
func (d *ddlAdapter) AddCompositeUnique(ctx context.Context, tenantID uuid.UUID, tableName string, columns []string) error {
	schema := schemaOf(tenantID)
	table := safeIdent(tableName)
	constraintName := safeIdent(tableName + "_" + strings.Join(columns, "_") + "_uq")

	cols := make([]string, len(columns))
	for i, c := range columns {
		cols[i] = safeIdent(c)
	}
	_, err := d.db.Exec(ctx, fmt.Sprintf(
		"ALTER TABLE %s.%s ADD CONSTRAINT %s UNIQUE (%s)",
		schema, table, constraintName, strings.Join(cols, ", "),
	))
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && (pgErr.Code == "42710" || pgErr.Code == "42P07") {
			return nil // constraint or relation already exists — idempotent
		}
		return err
	}
	return nil
}

// SetColumnUnique adds or removes the UNIQUE constraint on an existing column.
// When adding, Postgres will reject the call if duplicates already exist.
// The constraint name follows Postgres auto-naming: table_column_key.
func (d *ddlAdapter) SetColumnUnique(ctx context.Context, tenantID uuid.UUID, tableName, columnName string, add bool) error {
	schema := schemaOf(tenantID)
	table := safeIdent(tableName)
	col := safeIdent(columnName)

	if add {
		_, err := d.db.Exec(ctx, fmt.Sprintf(
			"ALTER TABLE %s.%s ADD UNIQUE (%s)", schema, table, col))
		return err
	}
	// Postgres auto-names inline UNIQUE constraints as table_column_key.
	constraintName := safeIdent(tableName + "_" + columnName + "_key")
	_, err := d.db.Exec(ctx, fmt.Sprintf(
		"ALTER TABLE %s.%s DROP CONSTRAINT IF EXISTS %s", schema, table, constraintName))
	return err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

var safeIdentRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// safeIdent validates and double-quotes a Postgres identifier.
// Panics if the name contains characters outside [a-zA-Z0-9_] so that DDL
// injection is impossible regardless of caller input.
func safeIdent(name string) string {
	if !safeIdentRe.MatchString(name) {
		panic(fmt.Sprintf("postgres ddl: unsafe identifier %q", name))
	}
	return `"` + name + `"`
}

// fieldColumnDef returns the column definition fragment for a field.
func fieldColumnDef(f collection.Field) string {
	col := safeIdent(f.Name) + " " + pgType(f)
	if f.Required {
		col += " NOT NULL"
	}
	if f.Unique {
		col += " UNIQUE"
	}
	return col
}

// pgType maps a domain FieldType to a Postgres column type.
// If the field already carries an explicit DBType it is used directly;
// otherwise a sensible default is derived from the FieldType.
func pgType(f collection.Field) string {
	if f.DBType != "" {
		return f.DBType
	}
	switch f.Type {
	case collection.FieldTypeString:
		return "VARCHAR(255)"
	case collection.FieldTypeText, collection.FieldTypeRichText:
		return "TEXT"
	case collection.FieldTypeInteger:
		return "BIGINT"
	case collection.FieldTypeFloat:
		return "DOUBLE PRECISION"
	case collection.FieldTypeDecimal:
		return "NUMERIC"
	case collection.FieldTypeBoolean:
		return "BOOLEAN"
	case collection.FieldTypeDate:
		return "DATE"
	case collection.FieldTypeDateTime, collection.FieldTypeTimestamp:
		return "TIMESTAMPTZ"
	case collection.FieldTypeTime:
		return "TIME"
	case collection.FieldTypeJSON:
		return "JSONB"
	case collection.FieldTypeUUID:
		return "UUID"
	case collection.FieldTypeRelation:
		return "UUID" // FK to another tenant-schema table
	case collection.FieldTypeFile, collection.FieldTypeFiles:
		return "TEXT" // stores S3 key or JSON array of keys
	case collection.FieldTypeSelect:
		return "VARCHAR(100)"
	case collection.FieldTypeMultiSel, collection.FieldTypeCSV:
		return "TEXT[]"
	case collection.FieldTypeHash:
		return "TEXT"
	case collection.FieldTypeIP:
		return "INET"
	case collection.FieldTypeGeometry:
		return "TEXT" // PostGIS geometry if extension available
	default:
		return "TEXT"
	}
}
