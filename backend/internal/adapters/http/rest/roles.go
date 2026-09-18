package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountRoles(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &roleHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/roles", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionRead)).Get("/{id}", h.getByID)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionDelete)).Delete("/{id}", h.delete)

		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionRead)).Get("/{id}/permissions", h.listPermissions)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualRoles, userdomain.ActionUpdate)).Put("/{id}/permissions", h.setPermissions)
	})
}

type roleHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

func (h *roleHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	page, limit := parsePage(r)

	roles, err := h.repos.Role.List(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, roles, int64(len(roles)), page, limit)
}

func (h *roleHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

	var body struct {
		Name        string  `json:"name"`
		Description string  `json:"description"`
		Icon        *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}

	role := &userdomain.Role{
		Base:        domain.NewBase(tenantID, &actorID),
		Name:        body.Name,
		Description: body.Description,
		Icon:        body.Icon,
	}

	if err := h.repos.Role.Create(r.Context(), tenantID, role); err != nil {
		h.log.Errorw("role create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, role)
}

func (h *roleHandler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	role, err := h.repos.Role.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if role == nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *roleHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	role, err := h.repos.Role.GetByID(r.Context(), tenantID, id)
	if err != nil || role == nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if role.IsSystem {
		writeError(w, http.StatusForbidden, "system roles cannot be modified")
		return
	}

	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
		Icon        *string `json:"icon"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.Name != nil {
		role.Name = *body.Name
	}
	if body.Description != nil {
		role.Description = *body.Description
	}
	if body.Icon != nil {
		role.Icon = body.Icon
	}
	role.Touch(&actorID)

	if err := h.repos.Role.Update(r.Context(), tenantID, role); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, role)
}

func (h *roleHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	role, err := h.repos.Role.GetByID(r.Context(), tenantID, id)
	if err != nil || role == nil {
		writeError(w, http.StatusNotFound, "role not found")
		return
	}
	if role.IsSystem {
		writeError(w, http.StatusForbidden, "system roles cannot be deleted")
		return
	}

	if err := h.repos.Role.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *roleHandler) listPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	page, limit := parsePage(r)

	perms, err := h.repos.Permission.GetForRole(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	paginated(w, perms, int64(len(perms)), page, limit)
}

type permissionInput struct {
	Collection string            `json:"collection"`
	Action     userdomain.Action `json:"action"`
	Fields     []string          `json:"fields"`
	Filter     any               `json:"filter"`
}

// PUT /roles/{id}/permissions — replaces all permissions for the role
func (h *roleHandler) setPermissions(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	roleID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var inputs []permissionInput
	if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	perms := make([]*userdomain.Permission, 0, len(inputs))
	for _, input := range inputs {
		perms = append(perms, &userdomain.Permission{
			Base:       domain.NewBase(tenantID, &actorID),
			RoleID:     roleID,
			Collection: input.Collection,
			Action:     input.Action,
			Fields:     input.Fields,
			Filter:     input.Filter,
		})
	}
	if err := h.repos.Permission.ReplaceForRole(r.Context(), tenantID, roleID, perms); err != nil {
		h.log.Errorw("permission replace failed", "err", err)
		writeError(w, http.StatusInternalServerError, "set permissions failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
