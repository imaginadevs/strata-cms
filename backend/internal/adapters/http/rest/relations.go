package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountRelations(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &relationHandler{repos: repos, log: log}
	p := repos.Permission
	r.Route("/collections/{collection}/relations", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{id}", h.delete)
	})
}

type relationHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

// GET /collections/{collection}/relations
func (h *relationHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	rels, err := h.repos.Relation.List(r.Context(), tenantID, collName)
	if err != nil {
		h.log.Errorw("relations list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if rels == nil {
		rels = []*collectiondomain.Relation{}
	}
	writeJSON(w, http.StatusOK, rels)
}

// POST /collections/{collection}/relations
func (h *relationHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	var body struct {
		Type              collectiondomain.RelationType `json:"type"`
		RelatedCollection string                        `json:"related_collection"`
		FieldName         string                        `json:"field_name"`
		DisplayName       string                        `json:"display_name"`
		Required          bool                          `json:"required"`
		OnDelete          string                        `json:"on_delete"`
		JunctionTable     string                        `json:"junction_table"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Type == "" || body.RelatedCollection == "" {
		writeError(w, http.StatusBadRequest, "type and related_collection are required")
		return
	}
	if body.RelatedCollection == collName {
		writeError(w, http.StatusBadRequest, "related_collection must differ from the current collection")
		return
	}
	if body.OnDelete == "" {
		body.OnDelete = "set_null"
	}
	// A NOT NULL FK column cannot use SET NULL on delete — Postgres will reject it.
	if body.Required && body.OnDelete == "set_null" {
		writeError(w, http.StatusBadRequest, "required fields cannot use on_delete=set_null; use cascade or restrict")
		return
	}

	rel := &collectiondomain.Relation{
		Base:     domain.NewBase(tenantID, &actorID),
		Type:     body.Type,
		OnDelete: body.OnDelete,
	}

	switch body.Type {

	// Many → One: FK on current collection
	case collectiondomain.RelationManyToOne:
		if body.FieldName == "" {
			writeError(w, http.StatusBadRequest, "field_name is required for many_to_one")
			return
		}
		rel.CollectionMany = collName
		rel.FieldMany = body.FieldName
		rel.CollectionOne = body.RelatedCollection

		if err := h.repos.Collection.AddForeignKey(r.Context(), tenantID,
			collName, body.FieldName, body.RelatedCollection, false, body.Required, body.OnDelete); err != nil {
			h.log.Errorw("many_to_one: add FK failed", "err", err)
			writeError(w, http.StatusInternalServerError, "could not add foreign key column: "+err.Error())
			return
		}
		h.ensureFieldMeta(r.Context(), tenantID, actorID, collName, body.FieldName, body.DisplayName, body.Required, body.RelatedCollection)

	// One → One: FK on current collection, unique
	case collectiondomain.RelationOneToOne:
		if body.FieldName == "" {
			writeError(w, http.StatusBadRequest, "field_name is required for one_to_one")
			return
		}
		rel.CollectionMany = collName
		rel.FieldMany = body.FieldName
		rel.CollectionOne = body.RelatedCollection

		if err := h.repos.Collection.AddForeignKey(r.Context(), tenantID,
			collName, body.FieldName, body.RelatedCollection, true, body.Required, body.OnDelete); err != nil {
			h.log.Errorw("one_to_one: add FK failed", "err", err)
			writeError(w, http.StatusInternalServerError, "could not add foreign key column: "+err.Error())
			return
		}
		h.ensureFieldMeta(r.Context(), tenantID, actorID, collName, body.FieldName, body.DisplayName, body.Required, body.RelatedCollection)

	// One → Many: FK on the RELATED collection
	case collectiondomain.RelationOneToMany:
		if body.FieldName == "" {
			writeError(w, http.StatusBadRequest, "field_name is required for one_to_many")
			return
		}
		rel.CollectionMany = body.RelatedCollection
		rel.FieldMany = body.FieldName
		rel.CollectionOne = collName

		if err := h.repos.Collection.AddForeignKey(r.Context(), tenantID,
			body.RelatedCollection, body.FieldName, collName, false, body.Required, body.OnDelete); err != nil {
			h.log.Errorw("one_to_many: add FK on related failed", "err", err)
			writeError(w, http.StatusInternalServerError, "could not add foreign key column on related collection: "+err.Error())
			return
		}
		// For one_to_many the FK column is on the related collection and references the current one
		h.ensureFieldMeta(r.Context(), tenantID, actorID, body.RelatedCollection, body.FieldName, body.DisplayName, body.Required, collName)

	// Many ↔ Many: dedicated junction table
	case collectiondomain.RelationManyToMany:
		jt := body.JunctionTable
		if jt == "" {
			jt = collName + "_" + body.RelatedCollection
		}
		rel.CollectionMany = collName
		rel.CollectionOne = body.RelatedCollection
		rel.JunctionTable = &jt
		rel.FieldMany = jt // placeholder so the NOT NULL DB constraint is satisfied

		if err := h.repos.Collection.CreateJunctionTable(r.Context(), tenantID,
			jt, collName, body.RelatedCollection); err != nil {
			h.log.Errorw("many_to_many: create junction table failed", "err", err)
			writeError(w, http.StatusInternalServerError, "could not create junction table: "+err.Error())
			return
		}

	default:
		writeError(w, http.StatusBadRequest, "unknown relation type; must be many_to_one, one_to_many, many_to_many or one_to_one")
		return
	}

	if err := h.repos.Relation.Create(r.Context(), tenantID, rel); err != nil {
		h.log.Errorw("relation metadata insert failed", "err", err)
		writeError(w, http.StatusInternalServerError, "relation DDL applied but metadata save failed")
		return
	}

	writeJSON(w, http.StatusCreated, rel)
}

// DELETE /collections/{collection}/relations/{id}
func (h *relationHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	relID, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid relation id")
		return
	}

	rels, err := h.repos.Relation.List(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}

	var rel *collectiondomain.Relation
	for _, c := range rels {
		if c.ID == relID {
			rel = c
			break
		}
	}
	if rel == nil {
		writeError(w, http.StatusNotFound, "relation not found")
		return
	}

	// Undo DDL — best-effort; log failures but don't block the metadata delete
	switch rel.Type {
	case collectiondomain.RelationManyToOne, collectiondomain.RelationOneToOne:
		_ = h.repos.Field.Delete(r.Context(), tenantID, rel.CollectionMany, rel.FieldMany)
		if err := h.repos.Collection.DropColumn(r.Context(), tenantID, rel.CollectionMany, rel.FieldMany); err != nil {
			h.log.Warnw("delete relation: drop FK column failed", "err", err, "field", rel.FieldMany)
		}

	case collectiondomain.RelationOneToMany:
		_ = h.repos.Field.Delete(r.Context(), tenantID, rel.CollectionMany, rel.FieldMany)
		if err := h.repos.Collection.DropColumn(r.Context(), tenantID, rel.CollectionMany, rel.FieldMany); err != nil {
			h.log.Warnw("delete relation: drop FK column on related failed", "err", err)
		}

	case collectiondomain.RelationManyToMany:
		if rel.JunctionTable != nil {
			if err := h.repos.Collection.DropTable(r.Context(), tenantID, *rel.JunctionTable); err != nil {
				h.log.Warnw("delete relation: drop junction table failed", "err", err)
			}
		}
	}

	if err := h.repos.Relation.Delete(r.Context(), tenantID, relID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ensureFieldMeta creates a Field metadata record for the FK column so that
// the Content page can display and edit it. Non-fatal — just logs on failure.
func (h *relationHandler) ensureFieldMeta(
	ctx context.Context,
	tenantID, actorID uuid.UUID,
	collName, fieldName, displayName string,
	required bool,
	relatedCollection string,
) {
	if displayName == "" {
		displayName = strings.Title( //nolint:staticcheck
			strings.ReplaceAll(strings.TrimSuffix(fieldName, "_id"), "_", " "),
		)
	}
	sec, err := h.repos.Section.GetByName(ctx, tenantID, collName, collectiondomain.DefaultSectionName)
	if err != nil || sec == nil {
		h.log.Warnw("ensureFieldMeta: general section missing", "err", err, "collection", collName)
		return
	}
	f := &collectiondomain.Field{
		Base:         domain.NewBase(tenantID, &actorID),
		SectionID:    sec.ID,
		SectionName:  sec.Name,
		Name:         fieldName,
		DisplayName:  displayName,
		Type:         collectiondomain.FieldTypeRelation,
		DBType:       "uuid",
		Required:     required,
		Options:      map[string]any{"related_collection": relatedCollection},
	}
	if err := h.repos.Field.Create(ctx, tenantID, f); err != nil {
		h.log.Warnw("ensureFieldMeta: create field metadata failed", "err", err, "field", fieldName)
	}
}
