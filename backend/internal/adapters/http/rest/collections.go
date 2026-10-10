package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountCollections(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &collectionHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/collections", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/reorder", h.reorderCollections)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/{name}", h.getByName)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/{name}", h.update)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{name}", h.delete)
	})
}

func MountFields(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &collectionHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/collections/{collection}/fields", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.listFields)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.createField)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/reorder", h.reorderFields)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/{name}", h.getField)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/{name}", h.updateField)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{name}", h.deleteField)
	})
}

type collectionHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

// ─── Collections ──────────────────────────────────────────────────────────────

// featureCollectionForbidden writes 404 when the collection belongs to a disabled feature.
func featureCollectionForbidden(w http.ResponseWriter, r *http.Request, name string) bool {
	settings := tenant.Settings{}
	if t, ok := httpMiddleware.TenantFromCtx(r.Context()); ok && t != nil {
		settings = t.Settings
	}
	if !features.CollectionAllowed(name, settings) {
		writeError(w, http.StatusNotFound, "collection not found")
		return true
	}
	return false
}

func (h *collectionHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	includeHidden := r.URL.Query().Get("hidden") == "true"
	page, limit := parsePage(r)

	cols, err := h.repos.Collection.List(r.Context(), tenantID, includeHidden)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	settings := tenant.Settings{}
	if t, ok := httpMiddleware.TenantFromCtx(r.Context()); ok && t != nil {
		settings = t.Settings
	}
	cols = features.FilterCollections(cols, settings)
	paginated(w, cols, int64(len(cols)), page, limit)
}

func (h *collectionHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

	var body struct {
		Name        string                          `json:"name"`
		DisplayName string                          `json:"display_name"`
		Type        collectiondomain.CollectionType `json:"type"`
		Icon        *string                         `json:"icon"`
		Color       *string                         `json:"color"`
		Note        *string                         `json:"note"`
		IsSingleton bool                            `json:"is_singleton"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	if userdomain.IsVirtualCollection(body.Name) {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%q is reserved for system permissions and cannot be created as a collection", body.Name))
		return
	}
	// Reject plugin prefixes (website_*), exact reserved names, and *_system.
	if reason, forbidden := features.IsForbiddenCollectionName(body.Name); forbidden {
		writeError(w, http.StatusUnprocessableEntity,
			fmt.Sprintf("%q cannot be created: %s", body.Name, reason))
		return
	}

	coll := collectiondomain.NewCollection(tenantID, body.Name, body.DisplayName, body.Type, &actorID)
	coll.Icon = body.Icon
	coll.Color = body.Color
	coll.Note = body.Note
	coll.IsSingleton = body.IsSingleton

	if coll.Type == collectiondomain.CollectionTypeAuth {
		coll.Fields = collectiondomain.AuthSystemFields()
	}

	// 1. Create the physical table; fail fast before any DB writes
	if err := h.repos.Collection.CreateTable(r.Context(), tenantID, coll); err != nil {
		h.log.Errorw("collection create table failed", "err", err, "name", coll.Name)
		writeError(w, http.StatusInternalServerError, "could not create table")
		return
	}

	// 2. Persist collection metadata; compensate by dropping the table on failure
	if err := h.repos.Collection.Create(r.Context(), tenantID, coll); err != nil {
		h.log.Errorw("collection metadata insert failed", "err", err)
		_ = h.repos.Collection.DropTable(r.Context(), tenantID, coll.Name)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// 3. Default section "general" (required invariant)
	general := collectiondomain.NewDefaultSection(tenantID, coll.Name, &actorID)
	if err := h.repos.Section.Create(r.Context(), tenantID, general); err != nil {
		h.log.Errorw("default section create failed", "err", err)
		_ = h.repos.Collection.Delete(r.Context(), tenantID, coll.Name)
		_ = h.repos.Collection.DropTable(r.Context(), tenantID, coll.Name)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// 4. Persist field metadata rows for auth-type system fields under general
	for i := range coll.Fields {
		f := &coll.Fields[i]
		f.Base = domain.NewBase(tenantID, &actorID)
		f.SectionID = general.ID
		f.SectionName = general.Name
		if err := h.repos.Field.Create(r.Context(), tenantID, f); err != nil {
			h.log.Warnw("system field metadata failed", "err", err, "field", f.Name)
		}
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventCollectionCreated,
		TenantID:   tenantID,
		Collection: coll.Name,
	})

	cid := coll.ID.String()
	recordAuditCtx(r.Context(), h.repos, h.log, r, "collections", &cid,
		auditdomain.OperationCreate, nil, collectionSnapshot(coll))

	// Reload with nested sections for the response
	if loaded, err := h.repos.Collection.GetByName(r.Context(), tenantID, coll.Name); err == nil && loaded != nil {
		coll = loaded
	}
	writeJSON(w, http.StatusCreated, coll)
}

// collectionSnapshot captures a collection's presentation/behaviour fields for
// audit old/new comparison (structural fields never change via this API).
func collectionSnapshot(c *collectiondomain.Collection) map[string]any {
	return map[string]any{
		"name":         c.Name,
		"display_name": c.DisplayName,
		"icon":         deref(c.Icon),
		"color":        deref(c.Color),
		"note":         deref(c.Note),
		"is_hidden":    c.IsHidden,
		"is_singleton": c.IsSingleton,
		"sort_field":   deref(c.SortField),
	}
}

func (h *collectionHandler) getByName(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")
	if featureCollectionForbidden(w, r, name) {
		return
	}

	coll, err := h.repos.Collection.GetByName(r.Context(), tenantID, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	writeJSON(w, http.StatusOK, coll)
}

func (h *collectionHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")

	coll, err := h.repos.Collection.GetByName(r.Context(), tenantID, name)
	if err != nil || coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	oldSnap := collectionSnapshot(coll)

	// Note: system collections are intentionally allowed here. This handler only
	// mutates presentation/behavior fields (display name, icon, colour, note,
	// hidden, singleton, sort) — never the table name, columns, or any DDL — so
	// hiding or relabelling a system/feature collection (e.g. an associative table
	// like pages_blocks) is safe. Structural protection lives on the field/DDL and
	// delete endpoints.

	var body struct {
		DisplayName *string         `json:"display_name"`
		Icon        *string         `json:"icon"`
		Color       *string         `json:"color"`
		Note        *string         `json:"note"`
		IsHidden    *bool           `json:"is_hidden"`
		IsSingleton *bool           `json:"is_singleton"`
		SortField   *string         `json:"sort_field"`
		GroupID     json.RawMessage `json:"group_id"` // absent = unchanged; null = ungroup; "uuid" = assign
		Sort        *int            `json:"sort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.DisplayName != nil {
		coll.DisplayName = *body.DisplayName
	}
	if body.Icon != nil {
		coll.Icon = body.Icon
	}
	if body.Color != nil {
		coll.Color = body.Color
	}
	if body.Note != nil {
		coll.Note = body.Note
	}
	if body.IsHidden != nil {
		coll.IsHidden = *body.IsHidden
	}
	if body.IsSingleton != nil {
		coll.IsSingleton = *body.IsSingleton
	}
	if body.SortField != nil {
		coll.SortField = body.SortField
	}
	if body.Sort != nil {
		coll.Sort = *body.Sort
	}
	if body.GroupID != nil { // key present in the request
		trimmed := strings.TrimSpace(string(body.GroupID))
		if trimmed == "null" || trimmed == `""` {
			coll.GroupID = nil // move to ungrouped
		} else {
			var gidStr string
			if err := json.Unmarshal(body.GroupID, &gidStr); err == nil && gidStr != "" {
				if gid, err := uuid.Parse(gidStr); err == nil {
					coll.GroupID = &gid
				}
			}
		}
	}
	coll.Touch(&actorID)

	if err := h.repos.Collection.Update(r.Context(), tenantID, coll); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	cid := coll.ID.String()
	recordAuditCtx(r.Context(), h.repos, h.log, r, "collections", &cid,
		auditdomain.OperationUpdate, oldSnap, collectionSnapshot(coll))

	writeJSON(w, http.StatusOK, coll)
}

// reorderCollections applies group membership + sort to many collections at once
// (sidebar / content-overview drag-and-drop).
func (h *collectionHandler) reorderCollections(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

	var body struct {
		Items []struct {
			ID      string  `json:"id"`
			GroupID *string `json:"group_id"`
			Sort    int     `json:"sort"`
		} `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	placements := make([]collectiondomain.CollectionPlacement, 0, len(body.Items))
	for _, it := range body.Items {
		id, err := uuid.Parse(it.ID)
		if err != nil {
			continue
		}
		var gid *uuid.UUID
		if it.GroupID != nil && *it.GroupID != "" {
			if g, err := uuid.Parse(*it.GroupID); err == nil {
				gid = &g
			}
		}
		placements = append(placements, collectiondomain.CollectionPlacement{ID: id, GroupID: gid, Sort: it.Sort})
	}
	if err := h.repos.Collection.ReorderCollections(r.Context(), tenantID, placements); err != nil {
		h.log.Errorw("reorder collections failed", "err", err)
		writeError(w, http.StatusInternalServerError, "reorder failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *collectionHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	name := chi.URLParam(r, "name")

	coll, err := h.repos.Collection.GetByName(r.Context(), tenantID, name)
	if err != nil || coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}
	if coll.IsSystem {
		writeError(w, http.StatusForbidden, "system collections cannot be deleted")
		return
	}

	// Delete metadata first (cascades to fields rows), then drop physical table
	if err := h.repos.Collection.Delete(r.Context(), tenantID, name); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if err := h.repos.Collection.DropTable(r.Context(), tenantID, name); err != nil {
		h.log.Errorw("drop table failed after metadata delete", "err", err, "collection", name)
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventCollectionDeleted,
		TenantID:   tenantID,
		Collection: name,
	})

	w.WriteHeader(http.StatusNoContent)
}

// ─── Fields ───────────────────────────────────────────────────────────────────

func (h *collectionHandler) listFields(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	page, limit := parsePage(r)

	fields, err := h.repos.Field.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, fields, int64(len(fields)), page, limit)
}

func (h *collectionHandler) createField(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	var body struct {
		Name         string                     `json:"name"`
		DisplayName  string                     `json:"display_name"`
		Type         collectiondomain.FieldType `json:"type"`
		DBType       string                     `json:"db_type"`
		Required     bool                       `json:"required"`
		Unique       bool                       `json:"unique"`
		Readonly     bool                       `json:"readonly"`
		Hidden       bool                       `json:"hidden"`
		Translatable bool                       `json:"translatable"`
		QuickEdit    bool                       `json:"quick_edit"`
		ShowInTable  bool                       `json:"show_in_table"`
		Sort         int                        `json:"sort"`
		Options      map[string]any             `json:"options"`
		Section      string                     `json:"section"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.Type == "" {
		writeError(w, http.StatusBadRequest, "name and type are required")
		return
	}
	sectionName := body.Section
	if sectionName == "" {
		sectionName = collectiondomain.DefaultSectionName
	}
	sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, sectionName)
	if err != nil || sec == nil {
		writeError(w, http.StatusUnprocessableEntity, "section not found")
		return
	}
	// Field names must be unique within the collection (physical columns).
	if existing, _ := h.repos.Field.GetByName(r.Context(), tenantID, collName, body.Name); existing != nil {
		writeError(w, http.StatusConflict, "field name already exists in this collection")
		return
	}

	f := &collectiondomain.Field{
		Base:         domain.NewBase(tenantID, &actorID),
		SectionID:    sec.ID,
		SectionName:  sec.Name,
		Name:         body.Name,
		DisplayName:  body.DisplayName,
		Type:         body.Type,
		DBType:       body.DBType,
		Required:     body.Required,
		Unique:       body.Unique,
		Readonly:     body.Readonly,
		Hidden:       body.Hidden,
		Translatable: body.Translatable,
		QuickEdit:    body.QuickEdit,
		ShowInTable:  body.ShowInTable,
		Sort:         body.Sort,
		Options:      body.Options,
	}

	// 1. Add column to physical table
	if err := h.repos.Collection.AddColumn(r.Context(), tenantID, collName, f); err != nil {
		h.log.Errorw("add column failed", "err", err, "field", f.Name)
		writeError(w, http.StatusInternalServerError, "could not add column")
		return
	}

	// 2. Persist field metadata; compensate by dropping the column on failure
	if err := h.repos.Field.Create(r.Context(), tenantID, f); err != nil {
		h.log.Errorw("field metadata insert failed", "err", err)
		_ = h.repos.Collection.DropColumn(r.Context(), tenantID, collName, f.Name)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, f)
}

func (h *collectionHandler) getField(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	f, err := h.repos.Field.GetByName(r.Context(), tenantID, collName, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if f == nil {
		writeError(w, http.StatusNotFound, "field not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *collectionHandler) updateField(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	f, err := h.repos.Field.GetByName(r.Context(), tenantID, collName, name)
	if err != nil || f == nil {
		writeError(w, http.StatusNotFound, "field not found")
		return
	}
	if f.IsSystem {
		writeError(w, http.StatusForbidden, "system fields cannot be modified")
		return
	}

	var body struct {
		DisplayName  *string        `json:"display_name"`
		Required     *bool          `json:"required"`
		Unique       *bool          `json:"unique"`
		Readonly     *bool          `json:"readonly"`
		Hidden       *bool          `json:"hidden"`
		Translatable *bool          `json:"translatable"`
		QuickEdit    *bool          `json:"quick_edit"`
		ShowInTable  *bool          `json:"show_in_table"`
		Sort         *int           `json:"sort"`
		Options      map[string]any `json:"options"`
		Section      *string        `json:"section"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Unique requires a DDL change — apply before touching metadata.
	if body.Unique != nil && *body.Unique != f.Unique {
		if err := h.repos.Collection.SetColumnUnique(r.Context(), tenantID, collName, f.Name, *body.Unique); err != nil {
			writeError(w, http.StatusInternalServerError, "could not update unique constraint: "+err.Error())
			return
		}
		f.Unique = *body.Unique
	}

	if body.DisplayName != nil {
		f.DisplayName = *body.DisplayName
	}
	if body.Required != nil {
		f.Required = *body.Required
	}
	if body.Readonly != nil {
		f.Readonly = *body.Readonly
	}
	if body.Hidden != nil {
		f.Hidden = *body.Hidden
	}
	if body.Translatable != nil {
		f.Translatable = *body.Translatable
	}
	if body.QuickEdit != nil {
		f.QuickEdit = *body.QuickEdit
	}
	if body.ShowInTable != nil {
		f.ShowInTable = *body.ShowInTable
	}
	if body.Sort != nil {
		f.Sort = *body.Sort
	}
	if body.Options != nil {
		// Merge: keeps keys not sent by the client (e.g. auto_generate)
		if f.Options == nil {
			f.Options = make(map[string]any)
		}
		for k, v := range body.Options {
			f.Options[k] = v
		}
	}
	if body.Section != nil && *body.Section != "" && *body.Section != f.SectionName {
		sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, *body.Section)
		if err != nil || sec == nil {
			writeError(w, http.StatusUnprocessableEntity, "section not found")
			return
		}
		f.SectionID = sec.ID
		f.SectionName = sec.Name
	}
	f.UpdatedBy = &actorID

	if err := h.repos.Field.Update(r.Context(), tenantID, f); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (h *collectionHandler) deleteField(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	f, err := h.repos.Field.GetByName(r.Context(), tenantID, collName, name)
	if err != nil || f == nil {
		writeError(w, http.StatusNotFound, "field not found")
		return
	}
	if f.IsSystem {
		writeError(w, http.StatusForbidden, "system fields cannot be deleted")
		return
	}

	if err := h.repos.Field.Delete(r.Context(), tenantID, collName, name); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if err := h.repos.Collection.DropColumn(r.Context(), tenantID, collName, name); err != nil {
		h.log.Errorw("drop column failed after metadata delete", "err", err, "field", name)
	}

	w.WriteHeader(http.StatusNoContent)
}

// PATCH /collections/{collection}/fields/reorder
// Body: { "order": ["field_name_1", "field_name_2", ...] }
// Sets the sort value of each named field to its index in the array.
// System fields included in the list are reordered too; omitted fields keep
// their existing sort value.
func (h *collectionHandler) reorderFields(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	var body struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Order) == 0 {
		writeError(w, http.StatusBadRequest, "order must be a non-empty array of field names")
		return
	}

	for i, name := range body.Order {
		f, err := h.repos.Field.GetByName(r.Context(), tenantID, collName, name)
		if err != nil || f == nil {
			continue // skip unknown names silently
		}
		f.Sort = i
		f.UpdatedBy = &actorID
		if err := h.repos.Field.Update(r.Context(), tenantID, f); err != nil {
			h.log.Warnw("reorder: update sort failed", "field", name, "err", err)
		}
	}

	// Return the refreshed field list so the client can update its cache
	fields, err := h.repos.Field.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, fields)
}
