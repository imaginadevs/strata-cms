package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountAudit(r chi.Router, repos *postgresrepo.Repos, log *logger.Logger) {
	h := &auditHandler{repos: repos, log: log}
	r.With(httpMiddleware.RequirePermission(repos.Permission, userdomain.VirtualAudit, userdomain.ActionRead)).Get("/audit", h.list)
}

type auditHandler struct {
	repos *postgresrepo.Repos
	log   *logger.Logger
}

func (h *auditHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	page, limit := parsePage(r)

	filter := outbound.AuditFilter{Page: page, Limit: limit}

	if coll := r.URL.Query().Get("collection"); coll != "" {
		filter.Collection = &coll
	}
	if userStr := r.URL.Query().Get("user_id"); userStr != "" {
		if uid, err := uuid.Parse(userStr); err == nil {
			filter.UserID = &uid
		}
	}
	if op := r.URL.Query().Get("operation"); op != "" {
		o := auditdomain.Operation(op)
		filter.Operation = &o
	}

	logs, total, err := h.repos.Audit.List(r.Context(), tenantID, filter)
	if err != nil {
		h.log.Errorw("audit list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, logs, total, page, limit)
}
