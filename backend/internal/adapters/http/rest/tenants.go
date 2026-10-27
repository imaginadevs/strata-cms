package rest

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/database"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountTenants registers tenant routes.
// publicR is the unauthenticated group (for /tenants/current).
// protectedR is the JWT group (for CRUD).
func MountTenants(publicR, protectedR chi.Router, repos *postgresrepo.Repos, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
	h := &tenantHandler{repos: repos, db: db, bus: bus, log: log}

	publicR.Get("/tenants/current", h.current)

	protectedR.Route("/tenants", func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.create)
		r.Get("/{id}", h.getByID)
		r.Patch("/{id}", h.update)
		r.Put("/{id}/plugins", h.putPlugins)
		r.Delete("/{id}", h.delete)
	})
}

type tenantHandler struct {
	repos *postgresrepo.Repos
	db    *pgxpool.Pool
	bus   events.Bus
	log   *logger.Logger
}

// tenantAdminEmail builds the seeded admin login from the tenant's domain
// (admin@<domain>), falling back to admin@<slug> when no domain is set.
func tenantAdminEmail(t *tenant.Tenant) string {
	host := ""
	if t.Domain != nil {
		host = strings.TrimSpace(*t.Domain)
	}
	if host == "" {
		host = t.Slug
	}
	return "admin@" + host
}

// GET /api/tenants/current — public; returns the resolved tenant for branding
func (h *tenantHandler) current(w http.ResponseWriter, r *http.Request) {
	t, ok := httpMiddleware.TenantFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusBadRequest, "tenant not resolved")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// GET /api/tenants — super admin: all; regular admin: own tenant
func (h *tenantHandler) list(w http.ResponseWriter, r *http.Request) {
	page, limit := parsePage(r)

	if httpMiddleware.IsSuperAdmin(r.Context()) {
		tenants, total, err := h.repos.Tenant.List(r.Context(), outbound.TenantFilter{Page: page, Limit: limit})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		paginated(w, tenants, total, page, limit)
		return
	}

	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	t, err := h.repos.Tenant.GetByID(r.Context(), tenantID)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	paginated(w, []*tenant.Tenant{t}, 1, 1, 1)
}

// POST /api/tenants — super admin only
func (h *tenantHandler) create(w http.ResponseWriter, r *http.Request) {
	if !httpMiddleware.IsSuperAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	var body struct {
		Name            string `json:"name"`
		Slug            string `json:"slug"`
		DefaultLanguage string `json:"default_language"`
		Domain          string `json:"domain"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" || body.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}

	lang := body.DefaultLanguage
	if lang == "" {
		lang = "en"
	}
	t := tenant.New(body.Name, body.Slug, lang)
	if body.Domain != "" {
		t.Domain = &body.Domain
	}

	if err := h.repos.Tenant.Create(r.Context(), t); err != nil {
		h.log.Errorw("tenant create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	// Provision the tenant immediately so it's usable without a server restart:
	// per-tenant schema + base tables, the seed admin, then any enabled feature
	// collections. (Previously this only happened at startup via bootstrap, so a
	// tenant created mid-run — or inserted by hand — was left unprovisioned.)
	migrator := database.NewTenantMigrator(h.db)
	if err := migrator.EnsureSchema(r.Context(), t.ID); err != nil {
		h.log.Errorw("tenant provision: schema failed", "tenant", t.ID, "err", err)
		// Roll back the row so a failed provision doesn't leave a half-created tenant.
		_ = h.repos.Tenant.Delete(r.Context(), t.ID)
		writeError(w, http.StatusInternalServerError, "tenant provisioning failed")
		return
	}
	if err := migrator.SeedDefaults(r.Context(), t.ID, tenantAdminEmail(t)); err != nil {
		h.log.Warnw("tenant provision: seed defaults failed", "tenant", t.ID, "err", err)
	}
	t.Settings.NormalizePlugins()
	for _, id := range features.EnabledPlugins(t.Settings) {
		if err := features.EnsureFeatureCollections(r.Context(), t.ID, id,
			h.repos.Collection, h.repos.Field, h.repos.Section); err != nil {
			h.log.Warnw("tenant provision: feature collections failed", "tenant", t.ID, "plugin", id, "err", err)
		}
		if err := features.EnsureFeatureRelations(r.Context(), t.ID, id, h.repos.Relation); err != nil {
			h.log.Warnw("tenant provision: feature relations failed", "tenant", t.ID, "plugin", id, "err", err)
		}
		if err := features.EnsureFeatureWorkflows(r.Context(), t.ID, id, h.repos.Workflow); err != nil {
			h.log.Warnw("tenant provision: feature workflows failed", "tenant", t.ID, "plugin", id, "err", err)
		}
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:     events.EventTenantCreated,
		TenantID: t.ID,
	})

	writeJSON(w, http.StatusCreated, t)
}

// GET /api/tenants/{id}
func (h *tenantHandler) getByID(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if !httpMiddleware.IsSuperAdmin(r.Context()) {
		tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
		if id != tenantID {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	t, err := h.repos.Tenant.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// PATCH /api/tenants/{id}
func (h *tenantHandler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if !httpMiddleware.IsSuperAdmin(r.Context()) {
		tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
		if id != tenantID {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
	}

	t, err := h.repos.Tenant.GetByID(r.Context(), id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}

	var body struct {
		Name            *string          `json:"name"`
		DefaultLanguage *string          `json:"default_language"`
		Domain          *string          `json:"domain"`
		Logo            *string          `json:"logo"`
		PrimaryColor    *string          `json:"primary_color"`
		Active          *bool            `json:"active"`
		Settings        *tenant.Settings `json:"settings"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.Name != nil {
		t.Name = *body.Name
	}
	if body.DefaultLanguage != nil {
		t.DefaultLanguage = *body.DefaultLanguage
	}
	if body.Domain != nil {
		t.Domain = body.Domain
	}
	if body.Logo != nil {
		t.Logo = body.Logo
	}
	if body.PrimaryColor != nil {
		t.PrimaryColor = body.PrimaryColor
	}
	if body.Active != nil {
		t.Active = *body.Active
	}
	if body.Settings != nil {
		incoming := *body.Settings
		incoming.NormalizePlugins()
		prev := t.Settings
		prev.NormalizePlugins()
		if httpMiddleware.IsSuperAdmin(r.Context()) {
			// Super-admin may set entitlement (allowed) and enabled.
			t.Settings = incoming
		} else {
			incoming.Plugins.Allowed = prev.Plugins.Allowed
			for id, on := range incoming.Plugins.Enabled {
				if on && !incoming.PluginAllowed(id) {
					writeError(w, http.StatusForbidden, "plugin "+id+" is not allowed for this tenant")
					return
				}
			}
			t.Settings = incoming
		}
		t.Settings.NormalizePlugins()
	}

	if err := h.repos.Tenant.Update(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// DELETE /api/tenants/{id} — super admin only
func (h *tenantHandler) delete(w http.ResponseWriter, r *http.Request) {
	if !httpMiddleware.IsSuperAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	// The platform tenant owns the super-admin account — deleting it would lock
	// everyone out of tenant management, so it is protected.
	if t, err := h.repos.Tenant.GetByID(r.Context(), id); err == nil && t != nil && t.IsPlatform {
		writeError(w, http.StatusBadRequest, "the platform tenant cannot be deleted")
		return
	}

	if err := h.repos.Tenant.Delete(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PUT /api/tenants/{id}/plugins — super-admin sets which plugins the tenant may use.
// Body: { "allowed": ["website","blog"], "enabled": { "website": true } } (enabled optional).
func (h *tenantHandler) putPlugins(w http.ResponseWriter, r *http.Request) {
	if !httpMiddleware.IsSuperAdmin(r.Context()) {
		writeError(w, http.StatusForbidden, "super admin required")
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	t, err := h.repos.Tenant.GetByID(r.Context(), id)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	var body struct {
		Allowed   []string `json:"allowed"`
		Installed []string `json:"installed"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	for _, a := range body.Allowed {
		if _, ok := features.CatalogByID(features.FeatureID(a)); !ok {
			writeError(w, http.StatusBadRequest, "unknown plugin "+a)
			return
		}
	}
	t.Settings.NormalizePlugins()
	t.Settings.Plugins.Allowed = body.Allowed
	if body.Installed != nil {
		for _, id := range body.Installed {
			if !t.Settings.PluginAllowed(id) {
				writeError(w, http.StatusBadRequest, "cannot mark "+id+" installed — not in allowed list")
				return
			}
			if _, ok := features.CatalogByID(features.FeatureID(id)); !ok {
				writeError(w, http.StatusBadRequest, "unknown plugin "+id)
				return
			}
		}
		t.Settings.Plugins.Installed = body.Installed
	} else {
		kept := make([]string, 0, len(t.Settings.Plugins.Installed))
		for _, id := range t.Settings.Plugins.Installed {
			if t.Settings.PluginAllowed(id) {
				kept = append(kept, id)
			}
		}
		t.Settings.Plugins.Installed = kept
	}
	t.Settings.NormalizePlugins()
	if err := h.repos.Tenant.Update(r.Context(), t); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":  t,
		"plugins": features.AllowedCatalog(t.Settings),
	})
}
