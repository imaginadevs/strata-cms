package http

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	"github.com/imaginacolombia/cms/internal/adapters/http/rest"
	"github.com/imaginacolombia/cms/internal/adapters/mail/smtp"
	pdfstub "github.com/imaginacolombia/cms/internal/adapters/pdf/stub"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/events"
	_ "github.com/imaginacolombia/cms/internal/core/ecommerce/tenants"
	_ "github.com/imaginacolombia/cms/internal/core/hooks/providers"
	"github.com/imaginacolombia/cms/internal/core/workflows"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
	"github.com/imaginacolombia/cms/internal/infrastructure/static"
)

// apiError writes a JSON error body for router-level responses (404 / 405) so the
// API surface stays JSON-only. Handlers use rest.writeError for the same shape.
func apiError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func NewRouter(ctx context.Context, cfg *config.Config, log *logger.Logger, db *pgxpool.Pool, bus events.Bus) http.Handler {
	repos := postgresrepo.NewRepos(db)

	var wfEngine *workflows.Engine
	if cfg.Features.Workflows {
		reg := workflows.NewRegistry()
		reg.RegisterTrigger(workflows.NewFieldChangedTrigger())
		reg.RegisterTrigger(workflows.NewScheduleTrigger())
		reg.RegisterAction(workflows.NewEmailWithPDFAction(smtp.New(cfg.SMTP), pdfstub.New()))
		wfEngine = workflows.NewEngine(repos.Workflow, repos.Item, repos.Tenant, reg, log, 4)
		wfEngine.Start(bus)
		workflows.NewScheduler(wfEngine).Start(ctx)
	}

	r := chi.NewRouter()

	// â”€â”€ Global middleware â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(httpMiddleware.ZapLogger(log))
	r.Use(httpMiddleware.Recoverer(log)) // JSON 500 on panic (never an empty body)
	r.Use(middleware.Compress(5))

	// Non-/api routes serve the embedded SPA (client-side routing) when a
	// frontend build is embedded in the binary; otherwise they return JSON 404
	// (API-only dev binary — the SPA is served by the Vite dev server there).
	// Unmatched /api/* routes still hit the /api subrouter's JSON 404 below.
	if spa, ok := static.Handler(); ok {
		r.NotFound(spa.ServeHTTP)
	} else {
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			apiError(w, http.StatusNotFound, "not found")
		})
	}
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		apiError(w, http.StatusMethodNotAllowed, "method not allowed")
	})

	// â”€â”€ API routes â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€â”€
	r.Route("/api", func(r chi.Router) {
		// Unmatched API routes/methods respond with JSON, not chi's plain text.
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			apiError(w, http.StatusNotFound, "not found")
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
			apiError(w, http.StatusMethodNotAllowed, "method not allowed")
		})

		// Public routes (no auth required).
		// TenantResolver resolves tenant from the Host header / domain.
		var publicR chi.Router
		r.Group(func(r chi.Router) {
			r.Use(httpMiddleware.TenantResolver(db))
			rest.MountHealth(r, db)
			rest.MountAuth(r, cfg, repos, bus, log)
			rest.MountHooks(r, repos)
			// Store ref so MountTenants can attach /tenants/current here
			publicR = r
		})

		// Protected routes (JWT required).
		// Authenticate runs first â†’ sets CtxKeyTenantID from JWT claims.
		// TenantResolver sees it already set and skips DB lookup.
		r.Group(func(r chi.Router) {
			r.Use(httpMiddleware.Authenticate(cfg.Auth.JWTSecret))
			r.Use(httpMiddleware.TenantResolver(db))
			r.Use(httpMiddleware.SuperAdminGate(cfg.SuperAdmin.AllowedIPs))
			r.Use(httpMiddleware.ActivityTracker())

			rest.MountPermissionCatalog(r, repos)
			rest.MountUsers(r, repos, bus, log)
			rest.MountRoles(r, repos, bus, log)
			rest.MountCollections(r, repos, bus, log)
			rest.MountFields(r, repos, bus, log)
			rest.MountSections(r, repos, bus, log)
			rest.MountRelations(r, repos, log)
			rest.MountIndexes(r, repos, log)
			rest.MountAudit(r, repos, log)
			rest.MountNotifications(r, repos, bus, log)
			if wfEngine != nil {
				rest.MountWorkflows(r, repos, wfEngine, log)
			}
			rest.MountSettings(r, repos, log)
			rest.MountPlugins(r, repos, bus, log)
			rest.MountAutoGen(r)

			// Tenant CRUD (protected half); /tenants/current is public
			rest.MountTenants(publicR, r, repos, bus, log)
		})

		// Items + ecommerce quoting use optional auth: an unauthenticated
		// request is treated as the built-in "Public" role by RequirePermission
		// (see httpMiddleware.checkPermission), so a storefront catalog with
		// Public read grants works without a JWT, while authenticated callers
		// (system users or "auth" principals like customers/operators) still
		// get their normal role-based access and row-level filters.
		r.Group(func(r chi.Router) {
			r.Use(httpMiddleware.AuthenticateOptional(cfg.Auth.JWTSecret))
			r.Use(httpMiddleware.TenantResolver(db))
			r.Use(httpMiddleware.SuperAdminGate(cfg.SuperAdmin.AllowedIPs))

			rest.MountItems(r, repos, bus, log)
			rest.MountEcommerce(r, repos, log)
		})

		// SSE â€” separate group: JWT comes via ?token= query param because the
		// browser EventSource API cannot set custom headers.
		r.Group(func(r chi.Router) {
			r.Use(httpMiddleware.TenantResolver(db))
			rest.MountSSE(r, cfg, bus, log)
		})

		// GraphQL â€” optional auth; ordering rationale same as protected group.
		if cfg.Features.GraphQL {
			r.Group(func(r chi.Router) {
				r.Use(httpMiddleware.AuthenticateOptional(cfg.Auth.JWTSecret))
				r.Use(httpMiddleware.TenantResolver(db))
				rest.MountGraphQL(r, repos, bus, log)
			})
		}
	})

	return r
}
