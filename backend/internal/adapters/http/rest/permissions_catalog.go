package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
)

// MountPermissionCatalog exposes the virtual system collections for the
// frontend permission editor. Any authenticated user may read it.
func MountPermissionCatalog(r chi.Router, _ *postgresrepo.Repos) {
	r.Get("/permissions/catalog", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"actions":  userdomain.AllActions(),
			"virtual":  userdomain.VirtualCatalog(),
			"note":     "Content collections come from GET /collections; use their name as permissions.collection. Action \"all\" grants every action on that collection.",
		})
	})
}