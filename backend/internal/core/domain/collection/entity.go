package collection

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/imaginacolombia/cms/internal/core/domain"
)

// CollectionType determines built-in behaviour applied to the collection.
type CollectionType string

const (
	// CollectionTypeBase is a standard user-defined collection.
	CollectionTypeBase CollectionType = "base"
	// CollectionTypeAuth adds system fields username + password_hash and wires
	// the collection into the authentication engine.
	CollectionTypeAuth CollectionType = "auth"
)

// DefaultSectionName is the slug of the section auto-created with every collection.
const DefaultSectionName = "general"

// Collection represents a user-defined data model (table).
// System collections (is_system=true) cannot be modified via the API.
type Collection struct {
	domain.Base

	Name        string         `json:"name"         db:"name"`
	DisplayName string         `json:"display_name" db:"display_name"`
	Type        CollectionType `json:"type"         db:"type"`
	Icon        *string        `json:"icon"         db:"icon"`
	Color       *string        `json:"color"        db:"color"`
	Note        *string        `json:"note"         db:"note"`
	IsSystem    bool           `json:"is_system"    db:"is_system"`
	IsHidden    bool           `json:"is_hidden"    db:"is_hidden"`
	IsSingleton bool           `json:"is_singleton" db:"is_singleton"`
	SortField   *string        `json:"sort_field"   db:"sort_field"`
	Meta        map[string]any `json:"meta"         db:"meta"`
	// GroupID optionally places this collection inside a CollectionGroup (sidebar
	// folder). Nil = ungrouped. Sort is the position within its group (or among
	// ungrouped collections) in the navigation.
	GroupID      *uuid.UUID    `json:"group_id"     db:"group_id"`
	Sort         int           `json:"sort"         db:"sort"`
	Sections     []Section     `json:"sections,omitempty"`
	Translations []Translation `json:"translations,omitempty"`

	// Fields is a transient flatten used by DDL/bootstrap before sections are
	// persisted. Never serialised in API responses — use Sections / AllFields().
	Fields []Field `json:"-"`
}

// AllFields returns every field across all sections, in section then field sort order.
func (c *Collection) AllFields() []Field {
	if len(c.Sections) == 0 {
		return c.Fields
	}
	var out []Field
	for _, s := range c.Sections {
		out = append(out, s.Fields...)
	}
	return out
}

// AuthSystemFields returns the mandatory system fields injected into every
// collection of type "auth". They are created automatically and cannot be
// deleted or renamed by the user.
func AuthSystemFields() []Field {
	return []Field{
		{Name: "username", DisplayName: "Username", Type: FieldTypeString, DBType: "varchar(255)", Required: true, Unique: true, IsSystem: true},
		{Name: "password_hash", DisplayName: "Password", Type: FieldTypeHash, DBType: "text", Required: false, Hidden: true, IsSystem: true},
		{Name: "last_login", DisplayName: "Last Login", Type: FieldTypeDateTime, DBType: "timestamptz", Required: false, IsSystem: true},
		{Name: "status", DisplayName: "Status", Type: FieldTypeSelect, DBType: "varchar(50)", Required: true, IsSystem: true,
			DefaultValue: "active",
			Options: map[string]any{
				"choices": []map[string]string{
					{"value": "active", "label": "Active"},
					{"value": "inactive", "label": "Inactive"},
					{"value": "suspended", "label": "Suspended"},
				},
			},
		},
	}
}

// Section groups fields within a collection for form layout and visibility rules.
type Section struct {
	domain.Base

	CollectionName string        `json:"collection"             db:"collection_name"`
	Name           string        `json:"name"                   db:"name"`
	DisplayName    string        `json:"display_name"           db:"display_name"`
	Note           *string       `json:"note,omitempty"         db:"note"`
	Sort           int           `json:"sort"                   db:"sort"`
	Visibility     *Condition    `json:"visibility"             db:"visibility"`
	Translations   []Translation `json:"translations,omitempty" db:"translations"`
	Fields         []Field       `json:"fields,omitempty"`
}

// NewDefaultSection returns the required "general" section for a collection.
func NewDefaultSection(tenantID uuid.UUID, collectionName string, actorID *uuid.UUID) *Section {
	return &Section{
		Base:           domain.NewBase(tenantID, actorID),
		CollectionName: collectionName,
		Name:           DefaultSectionName,
		DisplayName:    "General",
		Sort:           0,
		Translations:   []Translation{},
	}
}

// Field represents a column in a user-defined collection.
// Ownership is strict: Field → Section → Collection (no collection_name on Field).
type Field struct {
	domain.Base

	SectionID    uuid.UUID      `json:"-"             db:"section_id"`
	SectionName  string         `json:"section"       db:"-"`
	Name         string         `json:"name"          db:"name"`
	DisplayName  string         `json:"display_name"  db:"display_name"`
	Type         FieldType      `json:"type"          db:"type"`
	DBType       string         `json:"db_type"       db:"db_type"`
	Required     bool           `json:"required"      db:"required"`
	Unique       bool           `json:"unique"        db:"unique"`
	Readonly     bool           `json:"readonly"      db:"readonly"`
	Hidden       bool           `json:"hidden"         db:"hidden"`
	QuickEdit    bool           `json:"quick_edit"     db:"quick_edit"`
	ShowInTable  bool           `json:"show_in_table"  db:"show_in_table"`
	DefaultValue any            `json:"default_value"  db:"default_value"`
	Validation   *Validation    `json:"validation"    db:"validation"`
	Options      map[string]any `json:"options"       db:"options"`
	Sort         int            `json:"sort"          db:"sort"`
	IsSystem     bool           `json:"is_system"     db:"is_system"`
	Translatable bool           `json:"translatable"  db:"translatable"`
}

// FieldType enumerates supported field types in the model engine.
type FieldType string

const (
	FieldTypeString    FieldType = "string"
	FieldTypeText      FieldType = "text"
	FieldTypeRichText  FieldType = "rich_text"
	FieldTypeInteger   FieldType = "integer"
	FieldTypeFloat     FieldType = "float"
	FieldTypeDecimal   FieldType = "decimal"
	FieldTypeBoolean   FieldType = "boolean"
	FieldTypeDate      FieldType = "date"
	FieldTypeDateTime  FieldType = "datetime"
	FieldTypeTime      FieldType = "time"
	FieldTypeJSON      FieldType = "json"
	FieldTypeUUID      FieldType = "uuid"
	FieldTypeRelation  FieldType = "relation"
	FieldTypeFile      FieldType = "file"
	FieldTypeFiles     FieldType = "files"
	FieldTypeSelect    FieldType = "select"
	FieldTypeMultiSel  FieldType = "multiselect"
	FieldTypeHash      FieldType = "hash"
	FieldTypeIP        FieldType = "ip"
	FieldTypeTimestamp FieldType = "timestamp"
	FieldTypeCSV       FieldType = "csv"
	FieldTypeGeometry  FieldType = "geometry"
)

type Validation struct {
	Min    *float64 `json:"min,omitempty"`
	Max    *float64 `json:"max,omitempty"`
	Regex  *string  `json:"regex,omitempty"`
	Custom *string  `json:"custom,omitempty"`
}

// Translation stores the translated display name / note per language.
type Translation struct {
	Language    string `json:"language"`
	DisplayName string `json:"display_name"`
	Note        string `json:"note,omitempty"`
}

// Relation defines how two collections are connected.
type Relation struct {
	domain.Base

	CollectionMany string       `json:"collection_many" db:"collection_many"`
	FieldMany      string       `json:"field_many"      db:"field_many"`
	CollectionOne  string       `json:"collection_one"  db:"collection_one"`
	FieldOne       *string      `json:"field_one"       db:"field_one"`
	Type           RelationType `json:"type"            db:"type"`
	JunctionTable  *string      `json:"junction_table"  db:"junction_table"`
	OnDelete       string       `json:"on_delete"       db:"on_delete"`
}

// CollectionGroup is a sidebar folder that collections can be organised under.
// Groups are optional; a collection with GroupID == nil is "ungrouped".
type CollectionGroup struct {
	domain.Base

	Name  string  `json:"name"  db:"name"`
	Icon  *string `json:"icon"  db:"icon"`
	Color *string `json:"color" db:"color"`
	Sort  int     `json:"sort"  db:"sort"`
}

// CollectionPlacement is one collection's group + sort assignment, used to apply
// drag-and-drop reordering in a single batch.
type CollectionPlacement struct {
	ID      uuid.UUID  `json:"id"`
	GroupID *uuid.UUID `json:"group_id"`
	Sort    int        `json:"sort"`
}

type RelationType string

const (
	RelationManyToOne  RelationType = "many_to_one"
	RelationOneToMany  RelationType = "one_to_many"
	RelationManyToMany RelationType = "many_to_many"
	RelationOneToOne   RelationType = "one_to_one"
)

type IndexType string

const (
	IndexTypeBTree    IndexType = "btree"
	IndexTypeFullText IndexType = "fulltext"
)

type CollectionIndex struct {
	ID             uuid.UUID  `json:"id"`
	CollectionName string     `json:"collection_name"`
	Name           string     `json:"name"`
	Type           IndexType  `json:"type"`
	Fields         []string   `json:"fields"`
	CreatedAt      time.Time  `json:"created_at"`
	CreatedBy      *uuid.UUID `json:"created_by"`
}

func NewCollection(tenantID uuid.UUID, name, displayName string, colType CollectionType, actorID *uuid.UUID) *Collection {
	if colType == "" {
		colType = CollectionTypeBase
	}
	return &Collection{
		Base:        domain.NewBase(tenantID, actorID),
		Name:        name,
		DisplayName: displayName,
		Type:        colType,
		IsSystem:    false,
		IsHidden:    false,
		IsSingleton: false,
		Meta:        map[string]any{},
	}
}
