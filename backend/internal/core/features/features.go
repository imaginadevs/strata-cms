// Package features manages installable tenant plugins that each provision a fixed
// set of system collections when enabled.
//
// Plugins are identified by ID (website, blog, …). Collection names are prefixed
// with the plugin ID (website_pages, blog_posts) so user-created collections
// cannot collide. Names ending in "_system" are also reserved.
package features

import (
	"strings"

	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
)

// FeatureID identifies a tenant plugin (kept name for call-site compatibility).
type FeatureID string

const (
	FeatureWebsite   FeatureID = "website"
	FeatureBlog      FeatureID = "blog"
	FeatureEcommerce FeatureID = "ecommerce"
	FeatureCMMS      FeatureID = "cmms"
)

// Manifest describes a plugin in the product catalog.
type Manifest struct {
	ID          FeatureID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Prefix      string    `json:"prefix"` // collection name prefix, e.g. "website"
}

// Catalog is the product-wide list of plugins compiled into this binary.
func Catalog() []Manifest {
	return []Manifest{
		{ID: FeatureWebsite, Name: "Website", Description: "Pages and reusable content blocks", Prefix: "website"},
		{ID: FeatureBlog, Name: "Blog", Description: "Posts and post categories", Prefix: "blog"},
		{ID: FeatureEcommerce, Name: "Ecommerce", Description: "Products, inventory, customers, orders, and calculators", Prefix: "ecommerce"},
		{ID: FeatureCMMS, Name: "CMMS", Description: "Assets, inspection forms, tasks, and field submissions", Prefix: "cmms"},
	}
}

// CatalogByID returns a manifest or false.
func CatalogByID(id FeatureID) (Manifest, bool) {
	for _, m := range Catalog() {
		if m.ID == id {
			return m, true
		}
	}
	return Manifest{}, false
}

// ReservedNames maps a collection name to the plugin that owns it.
var ReservedNames = map[string]FeatureID{
	"website_pages":                FeatureWebsite,
	"website_blocks":               FeatureWebsite,
	"website_pages_blocks":         FeatureWebsite,
	"blog_posts":                   FeatureBlog,
	"blog_posts_categories":        FeatureBlog,
	"ecommerce_product_categories": FeatureEcommerce,
	"ecommerce_products":           FeatureEcommerce,
	"ecommerce_product_variants":   FeatureEcommerce,
	"ecommerce_locations":          FeatureEcommerce,
	"ecommerce_inventory_levels":   FeatureEcommerce,
	"ecommerce_customers":          FeatureEcommerce,
	"ecommerce_payment_methods":    FeatureEcommerce,
	"ecommerce_shipment_methods":   FeatureEcommerce,
	"ecommerce_tax_methods":        FeatureEcommerce,
	"ecommerce_orders":             FeatureEcommerce,
	"ecommerce_order_items":        FeatureEcommerce,
	"ecommerce_order_transactions": FeatureEcommerce,
	"ecommerce_order_shipments":    FeatureEcommerce,
	"cmms_assets":                  FeatureCMMS,
	"cmms_forms":                   FeatureCMMS,
	"cmms_form_versions":           FeatureCMMS,
	"cmms_operators":               FeatureCMMS,
	"cmms_tasks":                   FeatureCMMS,
	"cmms_submissions":             FeatureCMMS,
	// Legacy unprefixed names (blocked from create; migrate on ensure).
	"pages":            FeatureWebsite,
	"blocks":           FeatureWebsite,
	"pages_blocks":     FeatureWebsite,
	"posts":            FeatureBlog,
	"posts_categories": FeatureBlog,
}

// LegacyRenames maps pre-prefix collection names to the current names.
var LegacyRenames = map[string]string{
	"pages":            "website_pages",
	"blocks":           "website_blocks",
	"pages_blocks":     "website_pages_blocks",
	"posts":            "blog_posts",
	"posts_categories": "blog_posts_categories",
}

// IsReserved returns the owning plugin and true if the name is an exact reserved name.
func IsReserved(name string) (FeatureID, bool) {
	f, ok := ReservedNames[name]
	return f, ok
}

// IsForbiddenCollectionName reports whether a user may not create this collection name.
// Prefixes (website_*) are a soft convention — install omits collisions instead of
// blocking all prefixed names. We only hard-block exact blueprint names and *_system.
func IsForbiddenCollectionName(name string) (reason string, forbidden bool) {
	if _, ok := IsReserved(name); ok {
		return "name is reserved for a system plugin collection", true
	}
	if strings.HasSuffix(name, "_system") {
		return "names ending with _system are reserved", true
	}
	return "", false
}

// FeatureEnabled reports whether the plugin is enabled on the tenant.
func FeatureEnabled(f FeatureID, settings tenant.Settings) bool {
	return settings.PluginEnabled(string(f))
}

// FeatureAllowed reports whether the tenant is entitled to see/toggle the plugin.
func FeatureAllowed(f FeatureID, settings tenant.Settings) bool {
	return settings.PluginAllowed(string(f))
}

// CollectionAllowed reports whether a collection may be listed/accessed.
func CollectionAllowed(name string, settings tenant.Settings) bool {
	owner, reserved := IsReserved(name)
	if !reserved {
		return true
	}
	// Map legacy names to the same plugin gate.
	return FeatureEnabled(owner, settings)
}

// FilterCollections drops plugin-owned collections whose plugin is off.
func FilterCollections(cols []*collectiondomain.Collection, settings tenant.Settings) []*collectiondomain.Collection {
	out := make([]*collectiondomain.Collection, 0, len(cols))
	for _, c := range cols {
		if c == nil {
			continue
		}
		if CollectionAllowed(c.Name, settings) {
			out = append(out, c)
		}
	}
	return out
}

// EnabledPlugins returns catalog plugins that are installed (and allowed).
func EnabledPlugins(settings tenant.Settings) []FeatureID {
	var out []FeatureID
	for _, m := range Catalog() {
		if settings.PluginAllowed(string(m.ID)) && settings.PluginEnabled(string(m.ID)) {
			out = append(out, m.ID)
		}
	}
	return out
}

// PluginStatus is a catalog blueprint plus install state for the tenant UI.
type PluginStatus struct {
	Manifest
	Installed   bool     `json:"installed"`
	Collections []string `json:"collections"` // blueprint collection names
}

// AllowedCatalog returns blueprints this tenant may install (Install button).
func AllowedCatalog(settings tenant.Settings) []PluginStatus {
	var out []PluginStatus
	for _, m := range Catalog() {
		if !settings.PluginAllowed(string(m.ID)) {
			continue
		}
		names := make([]string, 0)
		for _, bp := range CollectionsForFeature(m.ID) {
			names = append(names, bp.Name)
		}
		out = append(out, PluginStatus{
			Manifest:    m,
			Installed:   settings.PluginInstalled(string(m.ID)),
			Collections: names,
		})
	}
	return out
}

// ─── Blueprints ───────────────────────────────────────────────────────────────

type Blueprint struct {
	Type            collectiondomain.CollectionType
	Name            string
	DisplayName     string
	Icon            string
	Color           string
	Hidden          bool
	Fields          []FieldBlueprint
	CompositeUnique [][]string
}

type FieldBlueprint struct {
	Name         string
	DisplayName  string
	Type         collectiondomain.FieldType
	DBType       string
	Required     bool
	Unique       bool
	DefaultValue any
	Options      map[string]any
}

func CollectionsForFeature(f FeatureID) []Blueprint {
	switch f {
	case FeatureWebsite:
		return websiteCollections()
	case FeatureBlog:
		return blogCollections()
	case FeatureEcommerce:
		return ecommerceCollections()
	case FeatureCMMS:
		return cmmsCollections()
	default:
		return nil
	}
}

type RelationSpec struct {
	CollectionMany string
	FieldMany      string
	CollectionOne  string
	Type           collectiondomain.RelationType
	OnDelete       string
}

func RelationsForFeature(f FeatureID) []RelationSpec {
	switch f {
	case FeatureWebsite:
		return []RelationSpec{
			{CollectionMany: "website_pages_blocks", FieldMany: "page_id", CollectionOne: "website_pages",
				Type: collectiondomain.RelationManyToOne, OnDelete: "cascade"},
			{CollectionMany: "website_pages_blocks", FieldMany: "block_id", CollectionOne: "website_blocks",
				Type: collectiondomain.RelationManyToOne, OnDelete: "restrict"},
		}
	case FeatureEcommerce:
		return ecommerceRelations()
	case FeatureCMMS:
		return cmmsRelations()
	default:
		return nil
	}
}

func websiteCollections() []Blueprint {
	return []Blueprint{
		{
			Name:            "website_pages",
			DisplayName:     "Pages",
			Icon:            "file-text",
			Color:           "#3b82f6",
			CompositeUnique: [][]string{{"slug", "language"}},
			Fields: []FieldBlueprint{
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "slug", DisplayName: "Slug", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "content", DisplayName: "Content", Type: collectiondomain.FieldTypeRichText, DBType: "text"},
				{
					Name: "status", DisplayName: "Status",
					Type: collectiondomain.FieldTypeSelect, DBType: "varchar(50)",
					Required: true, DefaultValue: "draft",
					Options: map[string]any{
						"choices": []map[string]string{
							{"value": "draft", "label": "Draft"},
							{"value": "published", "label": "Published"},
							{"value": "archived", "label": "Archived"},
						},
					},
				},
				{Name: "published_at", DisplayName: "Published At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
				{Name: "meta_title", DisplayName: "Meta Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "meta_description", DisplayName: "Meta Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
			},
		},
		{
			Name:            "website_blocks",
			DisplayName:     "Blocks",
			Icon:            "layout-grid",
			Color:           "#8b5cf6",
			CompositeUnique: [][]string{{"key", "language"}},
			Fields: []FieldBlueprint{
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "key", DisplayName: "Key", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
			},
		},
		{
			Name:        "website_pages_blocks",
			DisplayName: "Page Blocks",
			Icon:        "layers",
			Color:       "#6366f1",
			Hidden:      true,
			Fields: []FieldBlueprint{
				{Name: "page_id", DisplayName: "Page", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "block_id", DisplayName: "Block", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "content", DisplayName: "Content", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb"},
				{Name: "sort", DisplayName: "Sort", Type: collectiondomain.FieldTypeInteger, DBType: "integer", DefaultValue: 0},
			},
		},
	}
}

func blogCollections() []Blueprint {
	return []Blueprint{
		{
			Name:            "blog_posts_categories",
			DisplayName:     "Post Categories",
			Icon:            "folder",
			Color:           "#10b981",
			CompositeUnique: [][]string{{"slug", "language"}},
			Fields: []FieldBlueprint{
				{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "slug", DisplayName: "Slug", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "description", DisplayName: "Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
				{Name: "parent_id", DisplayName: "Parent Category", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
			},
		},
		{
			Name:            "blog_posts",
			DisplayName:     "Posts",
			Icon:            "newspaper",
			Color:           "#f59e0b",
			CompositeUnique: [][]string{{"slug", "language"}},
			Fields: []FieldBlueprint{
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "slug", DisplayName: "Slug", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "excerpt", DisplayName: "Excerpt", Type: collectiondomain.FieldTypeText, DBType: "text"},
				{Name: "content", DisplayName: "Content", Type: collectiondomain.FieldTypeRichText, DBType: "text"},
				{
					Name: "status", DisplayName: "Status",
					Type: collectiondomain.FieldTypeSelect, DBType: "varchar(50)",
					Required: true, DefaultValue: "draft",
					Options: map[string]any{
						"choices": []map[string]string{
							{"value": "draft", "label": "Draft"},
							{"value": "published", "label": "Published"},
							{"value": "archived", "label": "Archived"},
						},
					},
				},
				{Name: "category_id", DisplayName: "Category", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "featured_image", DisplayName: "Featured Image", Type: collectiondomain.FieldTypeFile, DBType: "text"},
				{Name: "published_at", DisplayName: "Published At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
				{Name: "meta_title", DisplayName: "Meta Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				{Name: "meta_description", DisplayName: "Meta Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
			},
		},
	}
}
