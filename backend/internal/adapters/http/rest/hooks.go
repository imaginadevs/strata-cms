package rest

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/hooks"
)

// MountHooks wires every hooks.RegisterRoute registration (see
// internal/core/hooks/providers) onto r under "/hooks". Intended for the
// public route group — most hook consumers (payment webhooks, etc.) are
// external callers with no CMS session, so hook handlers authenticate
// themselves (signature/secret validation) rather than relying on JWT auth.
func MountHooks(r chi.Router, repos *postgresrepo.Repos) {
	hooks.Mount(r, resolveHookTenant, hooks.Repos{
		Item:       repos.Item,
		Collection: repos.Collection,
		User:       repos.User,
		Role:       repos.Role,
		Permission: repos.Permission,
		Tenant:     repos.Tenant,
	})
}

func resolveHookTenant(r *http.Request) (uuid.UUID, bool) {
	return httpMiddleware.TenantIDFromCtx(r.Context())
}
