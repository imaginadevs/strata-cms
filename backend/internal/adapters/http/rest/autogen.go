package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	"github.com/imaginacolombia/cms/internal/core/autogen"
)

// MountAutoGen registers the auto-generation function discovery endpoint.
func MountAutoGen(r chi.Router) {
	r.Get("/autogen/functions", listAutoGenFunctions)
}

// GET /autogen/functions
// Returns the list of auto-generation functions available for the current tenant.
// Global built-ins are always present; tenant-specific additions are merged on top.
func listAutoGenFunctions(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	fns := autogen.List(tenantID)
	writeJSON(w, http.StatusOK, fns)
}
