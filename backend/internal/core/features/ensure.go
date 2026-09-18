package features

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/imaginacolombia/cms/internal/core/domain"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// InstallResult is returned by InstallPlugin (idempotent).
type InstallResult struct {
	Plugin             FeatureID `json:"plugin"`
	CollectionsCreated []string  `json:"collections_created"`
	CollectionsOmitted []Omit    `json:"collections_omitted"`
	RelationsCreated   []string  `json:"relations_created"`
	RelationsOmitted   []Omit    `json:"relations_omitted"`
}

// Omit explains why a blueprint item was skipped.
type Omit struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// InstallPlugin materializes a plugin blueprint: creates missing collections and
// relations. Already-present items are omitted (not errors). Idempotent.
// roleRepo/permRepo are used by plugins that provision RBAC defaults
// (ecommerce, cmms); pass nil to skip that step (e.g. from contexts that
// don't need it, such as tests exercising only schema provisioning).
func InstallPlugin(
	ctx context.Context,
	tenantID uuid.UUID,
	plugin FeatureID,
	collRepo outbound.CollectionRepository,
	fieldRepo outbound.FieldRepository,
	sectionRepo outbound.SectionRepository,
	relRepo outbound.RelationRepository,
	itemRepo outbound.ItemRepository,
	roleRepo outbound.RoleRepository,
	permRepo outbound.PermissionRepository,
) (InstallResult, error) {
	res := InstallResult{
		Plugin:             plugin,
		CollectionsCreated: []string{},
		CollectionsOmitted: []Omit{},
		RelationsCreated:   []string{},
		RelationsOmitted:   []Omit{},
	}
	if _, ok := CatalogByID(plugin); !ok {
		return res, fmt.Errorf("unknown plugin %q", plugin)
	}
	if err := MigrateLegacyCollections(ctx, tenantID, plugin, collRepo); err != nil {
		return res, err
	}

	for _, bp := range CollectionsForFeature(plugin) {
		created, err := ensureCollection(ctx, tenantID, bp, collRepo, fieldRepo, sectionRepo)
		if err != nil {
			return res, fmt.Errorf("plugin %q: collection %q: %w", plugin, bp.Name, err)
		}
		if created {
			res.CollectionsCreated = append(res.CollectionsCreated, bp.Name)
		} else {
			res.CollectionsOmitted = append(res.CollectionsOmitted, Omit{
				Name:   bp.Name,
				Reason: "already exists",
			})
		}
	}

	relRes, err := ensureRelationsDetailed(ctx, tenantID, plugin, relRepo)
	if err != nil {
		return res, err
	}
	res.RelationsCreated = relRes.created
	res.RelationsOmitted = relRes.omitted
	if plugin == FeatureEcommerce && itemRepo != nil {
		if err := SeedEcommerceDefaults(ctx, tenantID, itemRepo); err != nil {
			return res, fmt.Errorf("plugin %q: seed defaults: %w", plugin, err)
		}
		if roleRepo != nil && permRepo != nil {
			if err := SeedEcommerceRBAC(ctx, tenantID, roleRepo, permRepo, collRepo); err != nil {
				return res, fmt.Errorf("plugin %q: seed rbac: %w", plugin, err)
			}
		}
	}
	if plugin == FeatureCMMS && roleRepo != nil && permRepo != nil {
		if err := SeedCMMSRBAC(ctx, tenantID, roleRepo, permRepo, collRepo); err != nil {
			return res, fmt.Errorf("plugin %q: seed rbac: %w", plugin, err)
		}
	}
	return res, nil
}

// EnsureFeatureCollections remains for bootstrap; ignores omit details.
func EnsureFeatureCollections(
	ctx context.Context,
	tenantID uuid.UUID,
	feature FeatureID,
	collRepo outbound.CollectionRepository,
	fieldRepo outbound.FieldRepository,
	sectionRepo outbound.SectionRepository,
) error {
	if err := MigrateLegacyCollections(ctx, tenantID, feature, collRepo); err != nil {
		return err
	}
	for _, bp := range CollectionsForFeature(feature) {
		if _, err := ensureCollection(ctx, tenantID, bp, collRepo, fieldRepo, sectionRepo); err != nil {
			return fmt.Errorf("feature %q: ensure collection %q: %w", feature, bp.Name, err)
		}
	}
	return nil
}

func EnsureFeatureRelations(
	ctx context.Context,
	tenantID uuid.UUID,
	feature FeatureID,
	relRepo outbound.RelationRepository,
) error {
	_, err := ensureRelationsDetailed(ctx, tenantID, feature, relRepo)
	return err
}

type relDetail struct {
	created []string
	omitted []Omit
}

func ensureRelationsDetailed(
	ctx context.Context,
	tenantID uuid.UUID,
	feature FeatureID,
	relRepo outbound.RelationRepository,
) (relDetail, error) {
	out := relDetail{created: []string{}, omitted: []Omit{}}
	for _, sp := range RelationsForFeature(feature) {
		label := sp.CollectionMany + "." + sp.FieldMany + "→" + sp.CollectionOne
		existing, err := relRepo.List(ctx, tenantID, sp.CollectionMany)
		if err != nil {
			return out, fmt.Errorf("feature %q: list relations for %q: %w", feature, sp.CollectionMany, err)
		}
		found := false
		for _, e := range existing {
			if e.CollectionMany == sp.CollectionMany && e.FieldMany == sp.FieldMany && e.CollectionOne == sp.CollectionOne {
				found = true
				break
			}
		}
		if found {
			out.omitted = append(out.omitted, Omit{Name: label, Reason: "already exists"})
			continue
		}
		rel := &collectiondomain.Relation{
			Base:           domain.NewBase(tenantID, nil),
			CollectionMany: sp.CollectionMany,
			FieldMany:      sp.FieldMany,
			CollectionOne:  sp.CollectionOne,
			Type:           sp.Type,
			OnDelete:       sp.OnDelete,
		}
		if err := relRepo.Create(ctx, tenantID, rel); err != nil {
			return out, fmt.Errorf("feature %q: create relation %s: %w", feature, label, err)
		}
		out.created = append(out.created, label)
	}
	return out, nil
}

// ensureCollection returns created=true when a new collection was provisioned.
func ensureCollection(
	ctx context.Context,
	tenantID uuid.UUID,
	bp Blueprint,
	collRepo outbound.CollectionRepository,
	fieldRepo outbound.FieldRepository,
	sectionRepo outbound.SectionRepository,
) (created bool, err error) {
	existing, err := collRepo.GetByName(ctx, tenantID, bp.Name)
	if err != nil {
		return false, fmt.Errorf("check existing: %w", err)
	}

	if existing != nil {
		if err := ensureMissingFields(ctx, tenantID, bp, collRepo, fieldRepo, sectionRepo); err != nil {
			return false, err
		}
		return false, nil
	}

	icon := bp.Icon
	color := bp.Color
	collectionType := bp.Type
	if collectionType == "" {
		collectionType = collectiondomain.CollectionTypeBase
	}
	coll := &collectiondomain.Collection{
		Base:        domain.NewBase(tenantID, nil),
		Name:        bp.Name,
		DisplayName: bp.DisplayName,
		Type:        collectionType,
		Icon:        &icon,
		Color:       &color,
		IsSystem:    true,
		IsHidden:    bp.Hidden,
		IsSingleton: false,
		Meta:        map[string]any{},
	}

	if err := collRepo.CreateTable(ctx, tenantID, coll); err != nil {
		return false, fmt.Errorf("create table: %w", err)
	}

	if err := collRepo.Create(ctx, tenantID, coll); err != nil {
		_ = collRepo.DropTable(ctx, tenantID, coll.Name)
		return false, fmt.Errorf("persist metadata: %w", err)
	}

	general := collectiondomain.NewDefaultSection(tenantID, coll.Name, nil)
	if err := sectionRepo.Create(ctx, tenantID, general); err != nil {
		return false, fmt.Errorf("create default section: %w", err)
	}

	sortOffset := 0
	if collectionType == collectiondomain.CollectionTypeAuth {
		systemFields := collectiondomain.AuthSystemFields()
		for i, systemField := range systemFields {
			f := buildAuthSystemField(tenantID, general.ID, general.Name, systemField, i)
			if err := collRepo.AddColumn(ctx, tenantID, coll.Name, f); err != nil {
				return false, fmt.Errorf("add auth column %q: %w", f.Name, err)
			}
			if err := fieldRepo.Create(ctx, tenantID, f); err != nil {
				return false, fmt.Errorf("persist auth field %q: %w", f.Name, err)
			}
		}
		sortOffset = len(systemFields)
	}

	for i, fb := range bp.Fields {
		f := buildField(tenantID, general.ID, general.Name, fb, i+sortOffset)
		if err := collRepo.AddColumn(ctx, tenantID, coll.Name, f); err != nil {
			return false, fmt.Errorf("add column %q: %w", f.Name, err)
		}
		if err := fieldRepo.Create(ctx, tenantID, f); err != nil {
			return false, fmt.Errorf("persist field %q: %w", f.Name, err)
		}
	}

	for _, cols := range bp.CompositeUnique {
		if err := collRepo.AddCompositeUnique(ctx, tenantID, bp.Name, cols); err != nil {
			return false, fmt.Errorf("add composite unique %v: %w", cols, err)
		}
	}

	return true, nil
}

func ensureMissingFields(
	ctx context.Context,
	tenantID uuid.UUID,
	bp Blueprint,
	collRepo outbound.CollectionRepository,
	fieldRepo outbound.FieldRepository,
	sectionRepo outbound.SectionRepository,
) error {
	general, err := ensureGeneralSection(ctx, tenantID, bp.Name, sectionRepo)
	if err != nil {
		return err
	}

	existing, err := fieldRepo.ListByCollection(ctx, tenantID, bp.Name)
	if err != nil {
		return fmt.Errorf("list existing fields: %w", err)
	}

	present := make(map[string]bool, len(existing))
	for _, f := range existing {
		present[f.Name] = true
	}

	sortOffset := 0
	if bp.Type == collectiondomain.CollectionTypeAuth {
		systemFields := collectiondomain.AuthSystemFields()
		for i, systemField := range systemFields {
			if present[systemField.Name] {
				continue
			}
			f := buildAuthSystemField(tenantID, general.ID, general.Name, systemField, i)
			if err := collRepo.AddColumn(ctx, tenantID, bp.Name, f); err != nil {
				return fmt.Errorf("add auth column %q: %w", f.Name, err)
			}
			if err := fieldRepo.Create(ctx, tenantID, f); err != nil {
				return fmt.Errorf("persist auth field %q: %w", f.Name, err)
			}
		}
		sortOffset = len(systemFields)
	}

	for i, fb := range bp.Fields {
		if present[fb.Name] {
			continue
		}
		f := buildField(tenantID, general.ID, general.Name, fb, i+sortOffset)
		if err := collRepo.AddColumn(ctx, tenantID, bp.Name, f); err != nil {
			return fmt.Errorf("add column %q: %w", f.Name, err)
		}
		if err := fieldRepo.Create(ctx, tenantID, f); err != nil {
			return fmt.Errorf("persist field %q: %w", f.Name, err)
		}
	}

	for _, cols := range bp.CompositeUnique {
		if err := collRepo.AddCompositeUnique(ctx, tenantID, bp.Name, cols); err != nil {
			return fmt.Errorf("add composite unique %v: %w", cols, err)
		}
	}
	return nil
}

func ensureGeneralSection(
	ctx context.Context,
	tenantID uuid.UUID,
	collectionName string,
	sectionRepo outbound.SectionRepository,
) (*collectiondomain.Section, error) {
	sec, err := sectionRepo.GetByName(ctx, tenantID, collectionName, collectiondomain.DefaultSectionName)
	if err != nil {
		return nil, fmt.Errorf("get general section: %w", err)
	}
	if sec != nil {
		return sec, nil
	}
	sec = collectiondomain.NewDefaultSection(tenantID, collectionName, nil)
	if err := sectionRepo.Create(ctx, tenantID, sec); err != nil {
		return nil, fmt.Errorf("create general section: %w", err)
	}
	return sec, nil
}

func buildField(tenantID, sectionID uuid.UUID, sectionName string, fb FieldBlueprint, sort int) *collectiondomain.Field {
	return &collectiondomain.Field{
		Base:         domain.NewBase(tenantID, nil),
		SectionID:    sectionID,
		SectionName:  sectionName,
		Name:         fb.Name,
		DisplayName:  fb.DisplayName,
		Type:         fb.Type,
		DBType:       fb.DBType,
		Required:     fb.Required,
		Unique:       fb.Unique,
		DefaultValue: fb.DefaultValue,
		Options:      fb.Options,
		Sort:         sort,
		IsSystem:     false,
	}
}

func buildAuthSystemField(tenantID, sectionID uuid.UUID, sectionName string, systemField collectiondomain.Field, sort int) *collectiondomain.Field {
	f := buildField(tenantID, sectionID, sectionName, FieldBlueprint{
		Name: systemField.Name, DisplayName: systemField.DisplayName, Type: systemField.Type,
		DBType: systemField.DBType, Required: systemField.Required, Unique: systemField.Unique,
		DefaultValue: systemField.DefaultValue, Options: systemField.Options,
	}, sort)
	f.Hidden = systemField.Hidden
	f.Readonly = systemField.Readonly
	f.IsSystem = true
	return f
}

// MigrateLegacyCollections renames pre-prefix system tables (pages → website_pages, etc.).
func MigrateLegacyCollections(
	ctx context.Context,
	tenantID uuid.UUID,
	feature FeatureID,
	collRepo outbound.CollectionRepository,
) error {
	for oldName, newName := range LegacyRenames {
		owner, ok := ReservedNames[oldName]
		if !ok || owner != feature {
			continue
		}
		old, err := collRepo.GetByName(ctx, tenantID, oldName)
		if err != nil {
			return fmt.Errorf("legacy check %q: %w", oldName, err)
		}
		if old == nil {
			continue
		}
		neu, err := collRepo.GetByName(ctx, tenantID, newName)
		if err != nil {
			return fmt.Errorf("legacy target check %q: %w", newName, err)
		}
		if neu != nil {
			continue
		}
		if err := collRepo.RenameCollection(ctx, tenantID, oldName, newName); err != nil {
			return fmt.Errorf("rename %s → %s: %w", oldName, newName, err)
		}
	}
	return nil
}
