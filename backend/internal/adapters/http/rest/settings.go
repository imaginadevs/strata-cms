package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/crypto"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountSettings(r chi.Router, repos *postgresrepo.Repos, cfg *config.Config, enc *crypto.Encryptor, log *logger.Logger) {
	h := &settingsHandler{repos: repos, cfg: cfg, enc: enc, log: log}
	p := repos.Permission

	r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/settings", h.get)
	r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionUpdate)).Patch("/settings", h.update)
}

type settingsHandler struct {
	repos *postgresrepo.Repos
	cfg   *config.Config
	enc   *crypto.Encryptor
	log   *logger.Logger
}

// storageEnvInfo tells the UI whether a default (environment) S3 exists, so the
// "inherit the default credentials" option can be offered.
func (h *settingsHandler) storageEnvInfo() map[string]any {
	return map[string]any{
		"configured": h.cfg.Storage.Bucket != "",
		"bucket":     h.cfg.Storage.Bucket,
		"endpoint":   h.cfg.Storage.Endpoint,
		"has_cdn":    h.cfg.Storage.CDNBaseURL != "",
	}
}

// GET /api/settings — returns current tenant settings (S3 secret masked)
func (h *settingsHandler) get(w http.ResponseWriter, r *http.Request) {
	t, ok := httpMiddleware.TenantFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}
	t.Settings.NormalizePlugins()
	// Never return the S3 secret; expose only whether one is set.
	secretSet := t.Settings.Storage.SecretKey != ""
	t.Settings.Storage.SecretKey = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":   t,
		"settings": t.Settings,
		// Only plugins this tenant is entitled to see (super-admin sets allowed).
		"plugins":            features.AllowedCatalog(t.Settings),
		"storage_env":        h.storageEnvInfo(),
		"storage_secret_set": secretSet,
	})
}

// tenantConfigSnapshot captures the tenant's editable config for audit diffs.
func tenantConfigSnapshot(t *tenant.Tenant) map[string]any {
	return map[string]any{
		"name":             t.Name,
		"default_language": t.DefaultLanguage,
		"logo":             deref(t.Logo),
		"primary_color":    deref(t.PrimaryColor),
		"settings":         toMap(t.Settings),
	}
}

// PATCH /api/settings — updates tenant settings
func (h *settingsHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

	t, err := h.repos.Tenant.GetByID(r.Context(), tenantID)
	if err != nil || t == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	oldSnap := tenantConfigSnapshot(t)

	var body struct {
		Name            *string          `json:"name"`
		DefaultLanguage *string          `json:"default_language"`
		Logo            *string          `json:"logo"`
		PrimaryColor    *string          `json:"primary_color"`
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
	if body.Logo != nil {
		t.Logo = body.Logo
	}
	if body.PrimaryColor != nil {
		t.PrimaryColor = body.PrimaryColor
	}

	prev := t.Settings
	prev.NormalizePlugins()
	if body.Settings != nil {
		incoming := *body.Settings
		incoming.NormalizePlugins()
		// Tenant admins cannot expand entitlement — preserve Allowed from DB.
		incoming.Plugins.Allowed = prev.Plugins.Allowed
		// Reject enabling a plugin that is not allowed.
		for id, on := range incoming.Plugins.Enabled {
			if on && !incoming.PluginAllowed(id) {
				writeError(w, http.StatusForbidden, "plugin "+id+" is not allowed for this tenant")
				return
			}
		}
		// Sync legacy Features from Enabled for older clients posting features.*.
		if incoming.Features.Website || incoming.Features.Blog || incoming.Features.Ecommerce {
			if incoming.Plugins.Enabled == nil {
				incoming.Plugins.Enabled = map[string]bool{}
			}
			if incoming.Features.Website {
				incoming.Plugins.Enabled["website"] = true
			}
			if incoming.Features.Blog {
				incoming.Plugins.Enabled["blog"] = true
			}
			if incoming.Features.Ecommerce {
				incoming.Plugins.Enabled["ecommerce"] = true
			}
		}
		incoming.NormalizePlugins()
		// Preserve the stored (already-encrypted) S3 secret when the client didn't
		// re-enter it (GET masks it to ""); otherwise encrypt the new secret at rest.
		if incoming.Storage.SecretKey == "" {
			incoming.Storage.SecretKey = prev.Storage.SecretKey
		} else {
			incoming.Storage.SecretKey = h.enc.Encrypt(incoming.Storage.SecretKey)
		}
		t.Settings = incoming
	}

	if err := h.repos.Tenant.Update(r.Context(), t); err != nil {
		h.log.Errorw("settings update failed", "err", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	tid := tenantID.String()
	recordAuditCtx(r.Context(), h.repos, h.log, r, "settings", &tid,
		auditdomain.OperationUpdate, oldSnap, tenantConfigSnapshot(t))

	// Collection provisioning is explicit via POST /api/plugins/{id}/install.

	secretSet := t.Settings.Storage.SecretKey != ""
	t.Settings.Storage.SecretKey = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"tenant":             t,
		"settings":           t.Settings,
		"plugins":            features.AllowedCatalog(t.Settings),
		"storage_env":        h.storageEnvInfo(),
		"storage_secret_set": secretSet,
	})
}
