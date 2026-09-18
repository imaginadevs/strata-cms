package rest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/domain/ids"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountIndexes(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &indexHandler{repos: repos, log: log}
	p := repos.Permission
	r.Route("/collections/{collection}/indexes", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{id}", h.delete)
	})
}

type indexHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

// GET /collections/{collection}/indexes
func (h *indexHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	indexes, err := h.repos.Index.List(r.Context(), tenantID, collName)
	if err != nil {
		h.log.Errorw("indexes list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if indexes == nil {
		indexes = []*collectiondomain.CollectionIndex{}
	}
	writeJSON(w, http.StatusOK, indexes)
}

// POST /collections/{collection}/indexes
func (h *indexHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	var body struct {
		Type   collectiondomain.IndexType `json:"type"`
		Fields []string                   `json:"fields"`
		Name   string                     `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Type != collectiondomain.IndexTypeBTree && body.Type != collectiondomain.IndexTypeFullText {
		writeError(w, http.StatusBadRequest, "type must be btree or fulltext")
		return
	}
	if len(body.Fields) == 0 {
		writeError(w, http.StatusBadRequest, "at least one field is required")
		return
	}

	// Auto-generate name if not provided
	if body.Name == "" {
		suffix := "idx"
		if body.Type == collectiondomain.IndexTypeFullText {
			suffix = "ft_idx"
		}
		body.Name = buildIndexName(collName, body.Fields, suffix)
	}

	idx := &collectiondomain.CollectionIndex{
		ID:             ids.New(),
		CollectionName: collName,
		Name:           body.Name,
		Type:           body.Type,
		Fields:         body.Fields,
		CreatedAt:      time.Now(),
		CreatedBy:      &actorID,
	}

	// 1. Apply DDL — fail fast
	if err := h.repos.Collection.CreateIndex(r.Context(), tenantID, idx); err != nil {
		h.log.Errorw("create index DDL failed", "err", err, "name", idx.Name)
		writeError(w, http.StatusInternalServerError, "could not create index: "+err.Error())
		return
	}

	// 2. Persist metadata; compensate by dropping the index on failure
	if err := h.repos.Index.Create(r.Context(), tenantID, idx); err != nil {
		h.log.Errorw("index metadata insert failed", "err", err)
		_ = h.repos.Collection.DropIndex(r.Context(), tenantID, idx.Name)
		writeError(w, http.StatusInternalServerError, "index created but metadata save failed")
		return
	}

	writeJSON(w, http.StatusCreated, idx)
}

// DELETE /collections/{collection}/indexes/{id}
func (h *indexHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

	idxID, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid index id")
		return
	}

	idx, err := h.repos.Index.GetByID(r.Context(), tenantID, idxID)
	if err != nil {
		writeError(w, http.StatusNotFound, "index not found")
		return
	}

	// Best-effort DDL — log but don't block metadata deletion
	if err := h.repos.Collection.DropIndex(r.Context(), tenantID, idx.Name); err != nil {
		h.log.Warnw("drop index DDL failed", "err", err, "name", idx.Name)
	}

	if err := h.repos.Index.Delete(r.Context(), tenantID, idxID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// buildIndexName produces a Postgres-safe index name (max 63 chars).
func buildIndexName(collection string, fields []string, suffix string) string {
	name := fmt.Sprintf("%s_%s_%s", collection, strings.Join(fields, "_"), suffix)
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}
