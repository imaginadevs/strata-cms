package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	tenantdomain "github.com/imaginacolombia/cms/internal/core/domain/tenant"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountPlugins(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &pluginsHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission
	r.Route("/plugins", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionUpdate)).Post("/{id}/install", h.install)
	})
}

type pluginsHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

// GET /api/plugins — blueprints allowed for this tenant + installed flag.
func (h *pluginsHandler) list(w http.ResponseWriter, r *http.Request) {
	t, ok := httpMiddleware.TenantFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}
	t.Settings.NormalizePlugins()
	writeJSON(w, http.StatusOK, map[string]any{
		"data": features.AllowedCatalog(t.Settings),
	})
}

// POST /api/plugins/{id}/install — idempotent blueprint install.
// Creates missing collections/relations; omits ones that already exist.
func (h *pluginsHandler) install(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id := features.FeatureID(chi.URLParam(r, "id"))

	t, err := h.repos.Tenant.GetByID(r.Context(), tenantID)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	t.Settings.NormalizePlugins()
	if !t.Settings.PluginAllowed(string(id)) {
		writeError(w, http.StatusForbidden, "plugin not allowed for this tenant")
		return
	}
	if _, ok := features.CatalogByID(id); !ok {
		writeError(w, http.StatusNotFound, "unknown plugin")
		return
	}

	firstInstall := !t.Settings.PluginInstalled(string(id))

	result, err := features.InstallPlugin(r.Context(), tenantID, id,
		h.repos.Collection, h.repos.Field, h.repos.Section, h.repos.Relation, h.repos.Item,
		h.repos.Role, h.repos.Permission)
	if err != nil {
		h.log.Errorw("plugin install failed", "plugin", id, "err", err)
		writeError(w, http.StatusInternalServerError, "install failed: "+err.Error())
		return
	}

	t.Settings.MarkPluginInstalled(string(id))
	// Provision plugin-owned settings defaults on first activation only, so an
	// idempotent re-install never clobbers an operator's later changes.
	if id == features.FeatureEcommerce && firstInstall {
		t.Settings.Ecommerce = tenantdomain.DefaultEcommerceSettings()
	}
	if err := h.repos.Tenant.Update(r.Context(), t); err != nil {
		h.log.Errorw("plugin install: save installed flag failed", "err", err)
		writeError(w, http.StatusInternalServerError, "install recorded partially; settings update failed")
		return
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:     events.EventCollectionCreated,
		TenantID: tenantID,
		Payload: map[string]any{
			"plugin":              string(id),
			"collections_created": result.CollectionsCreated,
			"collections_omitted": result.CollectionsOmitted,
		},
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"result":   result,
		"settings": t.Settings,
		"plugins":  features.AllowedCatalog(t.Settings),
	})
}
