package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountAPITokens wires policy-scoped programmatic token management. Gated by the
// Settings permission (an admin-level action). The plaintext secret is returned
// ONLY by create and rotate; it is never retrievable afterwards.
func MountAPITokens(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &apiTokenHandler{repos: repos, log: log}
	p := repos.Permission
	r.Route("/api-tokens", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionUpdate)).Post("/{id}/rotate", h.rotate)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionDelete)).Delete("/{id}", h.delete)
	})
}

type apiTokenHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

func (h *apiTokenHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	tokens, err := h.repos.APIToken.List(r.Context(), tenantID)
	if err != nil {
		h.log.Errorw("api tokens list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if tokens == nil {
		tokens = []*apitoken.APIToken{}
	}
	writeJSON(w, http.StatusOK, tokens)
}

// POST /api-tokens
func (h *apiTokenHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())

	var body struct {
		Name     string            `json:"name"`
		Policies []apitoken.Policy `json:"policies"`
		Lifetime apitoken.Lifetime `json:"lifetime"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.Lifetime != apitoken.LifetimeForever && body.Lifetime != apitoken.LifetimeMonthly {
		body.Lifetime = apitoken.LifetimeForever
	}
	if body.Policies == nil {
		body.Policies = []apitoken.Policy{}
	}

	plaintext, prefix, hash, err := apitoken.GenerateSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate secret")
		return
	}

	tok := &apitoken.APIToken{
		Base:         domain.NewBase(tenantID, &actorID),
		Name:         body.Name,
		SecretPrefix: prefix,
		SecretHash:   hash,
		Policies:     body.Policies,
		Lifetime:     body.Lifetime,
		RotateAt:     apitoken.RotateAtFor(body.Lifetime, time.Now().UTC()),
	}
	if err := h.repos.APIToken.Create(r.Context(), tenantID, tok); err != nil {
		h.log.Errorw("api token create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// The plaintext secret is returned exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{
		"token":  tok,
		"secret": plaintext,
	})
}

// PATCH /api-tokens/{id} — edit name / policies / lifetime / revoked (never the secret)
func (h *apiTokenHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tok, err := h.repos.APIToken.GetByID(r.Context(), tenantID, id)
	if err != nil || tok == nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}

	var body struct {
		Name     *string            `json:"name"`
		Policies *[]apitoken.Policy `json:"policies"`
		Lifetime *apitoken.Lifetime `json:"lifetime"`
		Revoked  *bool              `json:"revoked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != nil {
		tok.Name = *body.Name
	}
	if body.Policies != nil {
		tok.Policies = *body.Policies
	}
	if body.Lifetime != nil && (*body.Lifetime == apitoken.LifetimeForever || *body.Lifetime == apitoken.LifetimeMonthly) {
		if *body.Lifetime != tok.Lifetime {
			tok.Lifetime = *body.Lifetime
			tok.RotateAt = apitoken.RotateAtFor(*body.Lifetime, time.Now().UTC())
			tok.LastNotifiedAt = nil
		}
	}
	if body.Revoked != nil {
		tok.Revoked = *body.Revoked
	}
	tok.UpdatedBy = &actorID

	if err := h.repos.APIToken.Update(r.Context(), tenantID, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, tok)
}

// POST /api-tokens/{id}/rotate — body {"mode":"now"|"grace"}
// "grace": issue a new secret now and keep the old valid for 24h.
// "now": issue a new secret and invalidate the old immediately.
func (h *apiTokenHandler) rotate(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	tok, err := h.repos.APIToken.GetByID(r.Context(), tenantID, id)
	if err != nil || tok == nil {
		writeError(w, http.StatusNotFound, "token not found")
		return
	}

	var body struct {
		Mode string `json:"mode"` // "now" | "grace"
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	plaintext := tok.Rotate(body.Mode == "grace", time.Now().UTC())
	tok.UpdatedBy = &actorID

	if err := h.repos.APIToken.Update(r.Context(), tenantID, tok); err != nil {
		writeError(w, http.StatusInternalServerError, "rotate failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":  tok,
		"secret": plaintext,
	})
}

func (h *apiTokenHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.repos.APIToken.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
