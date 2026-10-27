package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountSuperAdmin exposes a read-only view of the caller's super-admin status.
//
// The super-admin IP allowlist exists ONLY to gate tenant management, and it is
// configured in the environment (SUPER_ADMIN_ALLOWED_IPS) — never stored in the
// database and never editable through the API, so it cannot be escalated by any
// tenant or user action. The admin UI calls this endpoint so it can show the
// tenant-management features only when they will actually work (the account is a
// super admin AND the request comes from an allowed IP). That keeps a tenant
// admin from being offered actions the backend will then refuse with a 403.
func MountSuperAdmin(r chi.Router, cfg *config.Config, log *logger.Logger) {
	r.Get("/super-admin/status", func(w http.ResponseWriter, r *http.Request) {
		// claim: the account is flagged super admin (token `sa` claim / role).
		claim := httpMiddleware.IsSuperAdminFromCtx(r.Context())
		// active: claim AND the request passed the IP gate — this is what every
		// tenant-management handler actually enforces.
		active := httpMiddleware.IsSuperAdmin(r.Context())

		resp := map[string]any{
			"is_super_admin":       claim,
			"active":               active,
			"client_ip":            httpMiddleware.ClientIP(r),
			"allowlist_configured": len(cfg.SuperAdmin.AllowedIPs) > 0,
		}
		// Only reveal the configured allowlist to a caller who is already elevated
		// from an allowed IP — never to a non-elevated or non-super caller.
		if active {
			resp["allowed_ips"] = cfg.SuperAdmin.AllowedIPs
		}
		writeJSON(w, http.StatusOK, resp)
	})
}
