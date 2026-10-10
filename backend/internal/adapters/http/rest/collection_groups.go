package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountCollectionGroups wires the sidebar-folder (collection group) CRUD + reorder.
func MountCollectionGroups(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &collectionGroupHandler{repos: repos, log: log}
	p := repos.Permission
	r.Route("/collection-groups", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/reorder", h.reorder)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{id}", h.delete)
	})
}

type collectionGroupHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

func (h *collectionGroupHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	groups, err := h.repos.CollectionGroup.List(r.Context(), tenantID)
	if err != nil {
		h.log.Errorw("collection groups list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if groups == nil {
		groups = []*collectiondomain.CollectionGroup{}
	}
	writeJSON(w, http.StatusOK, groups)
}

func (h *collectionGroupHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

	var body struct {
		Name  string  `json:"name"`
		Icon  *string `json:"icon"`
		Color *string `json:"color"`
		Sort  int     `json:"sort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	g := &collectiondomain.CollectionGroup{
		Base:  domain.NewBase(tenantID, &actorID),
		Name:  body.Name,
		Icon:  body.Icon,
		Color: body.Color,
		Sort:  body.Sort,
	}
	if err := h.repos.CollectionGroup.Create(r.Context(), tenantID, g); err != nil {
		h.log.Errorw("collection group create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *collectionGroupHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	g, err := h.repos.CollectionGroup.GetByID(r.Context(), tenantID, id)
	if err != nil || g == nil {
		writeError(w, http.StatusNotFound, "group not found")
		return
	}

	var body struct {
		Name  *string `json:"name"`
		Icon  *string `json:"icon"`
		Color *string `json:"color"`
		Sort  *int    `json:"sort"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil {
		g.Name = *body.Name
	}
	if body.Icon != nil {
		g.Icon = body.Icon
	}
	if body.Color != nil {
		g.Color = body.Color
	}
	if body.Sort != nil {
		g.Sort = *body.Sort
	}
	g.Touch(&actorID)

	if err := h.repos.CollectionGroup.Update(r.Context(), tenantID, g); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *collectionGroupHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	// Member collections fall back to ungrouped (FK ON DELETE SET NULL).
	if err := h.repos.CollectionGroup.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *collectionGroupHandler) reorder(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	var body struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ids := make([]uuid.UUID, 0, len(body.Order))
	for _, s := range body.Order {
		if id, err := uuid.Parse(s); err == nil {
			ids = append(ids, id)
		}
	}
	if err := h.repos.CollectionGroup.Reorder(r.Context(), tenantID, ids); err != nil {
		writeError(w, http.StatusInternalServerError, "reorder failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
