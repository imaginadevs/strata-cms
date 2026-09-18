package providers

import (
	"net/http"

	"github.com/imaginacolombia/cms/internal/core/hooks"
)

func init() {
	// Generic (nil scope): available for every tenant Host.
	// Resulting URL: POST /api/hooks/example
	hooks.RegisterRoute(http.MethodPost, "/example", nil, func(w http.ResponseWriter, r *http.Request, hc hooks.HookContext) {
		_ = hc
		w.WriteHeader(http.StatusNoContent)
	})
}
