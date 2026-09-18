package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountUsers(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &userHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/users", func(r chi.Router) {
		// Self profile — no cms_users permission required (authenticated only).
		r.Get("/me", h.getMe)
		r.Patch("/me", h.updateMe)
		r.Post("/me/password", h.setMyPassword)

		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualUsers, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualUsers, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualUsers, userdomain.ActionRead)).Get("/{id}", h.getByID)
		// Self may PATCH own record without cms_users update; handler restricts fields.
		r.With(httpMiddleware.RequireUsersMutate(p, userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequireUsersMutate(p, userdomain.ActionDelete)).Delete("/{id}", h.delete)
		r.With(httpMiddleware.RequireUsersMutate(p, userdomain.ActionUpdate)).Post("/{id}/password", h.setPassword)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualUsers, userdomain.ActionUpdate)).Post("/{id}/roles/{roleId}", h.assignRole)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualUsers, userdomain.ActionUpdate)).Delete("/{id}/roles/{roleId}", h.revokeRole)
	})
}

type userHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

func (h *userHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	page, limit := parsePage(r)

	filter := outbound.UserFilter{Page: page, Limit: limit}
	if s := r.URL.Query().Get("search"); s != "" {
		filter.Search = s
	}

	users, total, err := h.repos.User.List(r.Context(), tenantID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, users, total, page, limit)
}

func (h *userHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

	var body struct {
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Password  string `json:"password"`
		Language  string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	u := userdomain.New(tenantID, body.Email, body.FirstName, body.LastName, userdomain.ProviderLocal)
	u.CreatedBy = &actorID
	u.UpdatedBy = &actorID
	if body.Language != "" {
		u.Language = body.Language
	}

	if body.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not hash password")
			return
		}
		h := string(hash)
		u.PasswordHash = &h
	}

	if err := h.repos.User.Create(r.Context(), tenantID, u); err != nil {
		h.log.Errorw("user create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	u.PasswordHash = nil // never return hash
	writeJSON(w, http.StatusCreated, u)
}

func (h *userHandler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	u, err := h.repos.User.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	u.PasswordHash = nil
	writeJSON(w, http.StatusOK, u)
}

func (h *userHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	u, err := h.repos.User.GetByID(r.Context(), tenantID, id)
	if err != nil || u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var body struct {
		FirstName *string            `json:"first_name"`
		LastName  *string            `json:"last_name"`
		Avatar    *string            `json:"avatar"`
		Status    *userdomain.Status `json:"status"`
		Language  *string            `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	canAdminUsers := userCanManageUsers(r, h.repos.Permission, tenantID, actorID)
	if actorID == id && !canAdminUsers {
		// Self without cms_users update: only profile fields (no avatar/status).
		if body.Avatar != nil || body.Status != nil {
			writeError(w, http.StatusForbidden, "cannot change avatar or status on own profile without users permission")
			return
		}
	}

	if body.FirstName != nil {
		u.FirstName = *body.FirstName
	}
	if body.LastName != nil {
		u.LastName = *body.LastName
	}
	if body.Avatar != nil {
		u.Avatar = body.Avatar
	}
	if body.Status != nil {
		u.Status = *body.Status
	}
	if body.Language != nil {
		u.Language = *body.Language
	}
	u.Touch(&actorID)

	if err := h.repos.User.Update(r.Context(), tenantID, u); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	u.PasswordHash = nil
	writeJSON(w, http.StatusOK, u)
}

func (h *userHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repos.User.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *userHandler) setPassword(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	if err := h.repos.User.SetPassword(r.Context(), tenantID, id, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "set password failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *userHandler) assignRole(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "roleId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid role id")
		return
	}

	if err := h.repos.Role.AssignToUser(r.Context(), tenantID, userID, roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "assign failed")
		return
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:     events.EventUserCreated,
		TenantID: tenantID,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (h *userHandler) revokeRole(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	roleID, err := uuid.Parse(chi.URLParam(r, "roleId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid role id")
		return
	}

	if err := h.repos.Role.RevokeFromUser(r.Context(), tenantID, userID, roleID); err != nil {
		writeError(w, http.StatusInternalServerError, "revoke failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}


// ─── Self profile (/users/me) ─────────────────────────────────────────────────

func (h *userHandler) getMe(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.repos.User.GetByID(r.Context(), tenantID, userID)
	if err != nil || u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	u.PasswordHash = nil
	writeJSON(w, http.StatusOK, u)
}

// updateMe allows first_name, last_name, language only (no avatar/status/email).
func (h *userHandler) updateMe(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	u, err := h.repos.User.GetByID(r.Context(), tenantID, userID)
	if err != nil || u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	var body struct {
		FirstName *string `json:"first_name"`
		LastName  *string `json:"last_name"`
		Language  *string `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.FirstName != nil {
		u.FirstName = *body.FirstName
	}
	if body.LastName != nil {
		u.LastName = *body.LastName
	}
	if body.Language != nil {
		u.Language = *body.Language
	}
	u.Touch(&userID)

	if err := h.repos.User.Update(r.Context(), tenantID, u); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	u.PasswordHash = nil
	writeJSON(w, http.StatusOK, u)
}

func (h *userHandler) setMyPassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Reuse setPassword by rewriting URL param — call logic inline.
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	if len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	if err := h.repos.User.SetPassword(r.Context(), tenantID, userID, string(hash)); err != nil {
		writeError(w, http.StatusInternalServerError, "set password failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func userCanManageUsers(r *http.Request, perms outbound.PermissionRepository, tenantID, userID uuid.UUID) bool {
	if httpMiddleware.IsSuperAdmin(r.Context()) || httpMiddleware.IsSuperAdminFromCtx(r.Context()) {
		return true
	}
	for _, role := range httpMiddleware.RolesFromCtx(r.Context()) {
		if role == "Administrator" {
			return true
		}
	}
	p, err := perms.GetForUser(r.Context(), tenantID, userID, userdomain.VirtualUsers, userdomain.ActionUpdate)
	return err == nil && p != nil
}
