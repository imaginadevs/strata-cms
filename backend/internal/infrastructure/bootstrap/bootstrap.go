// Package bootstrap runs once after migrations on every server start.
// All operations are idempotent — safe to leave configured in production.
package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	tenantdomain "github.com/imaginacolombia/cms/internal/core/domain/tenant"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/database"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// Run performs all startup bootstrap tasks:
//  1. For every active tenant, ensures the per-tenant schema exists and
//     is up-to-date via TenantMigrator.
//  2. Provisions system collections for any tenant that has feature flags enabled.
//
// Global goose migrations (public.tenants) are run by database.Migrate in main.go
// before this function is called.
func Run(ctx context.Context, cfg *config.Config, db *pgxpool.Pool, log *logger.Logger) error {
	migrator := database.NewTenantMigrator(db)
	repos := postgresrepo.NewRepos(db)

	// 1. Per-tenant schema migrations.
	active := true
	tenants, _, err := repos.Tenant.List(ctx, outbound.TenantFilter{Active: &active, Limit: 10_000})
	if err != nil {
		log.Errorw("bootstrap: list tenants failed", "err", err)
	}
	for _, t := range tenants {
		if err := migrator.EnsureSchema(ctx, t.ID); err != nil {
			log.Errorw("bootstrap: tenant schema migration failed", "tenant", t.ID, "err", err)
			continue
		}
		if err := migrator.SeedDefaults(ctx, t.ID, adminEmailFor(t)); err != nil {
			log.Errorw("bootstrap: tenant default seed failed", "tenant", t.ID, "err", err)
		}
	}

	// 2. Feature collections.
	if err := ensureAllFeatureCollections(ctx, repos, log); err != nil {
		log.Errorw("bootstrap: feature collection provisioning had errors", "err", err)
	}

	return nil
}

// adminEmailFor builds the seeded admin login from the tenant's domain
// (e.g. admin@acme.imaginadevs.com), falling back to the slug when no domain
// is set (admin@<slug>).
func adminEmailFor(t *tenantdomain.Tenant) string {
	host := ""
	if t.Domain != nil {
		host = strings.TrimSpace(*t.Domain)
	}
	if host == "" {
		host = t.Slug
	}
	return "admin@" + host
}

// ensureAllFeatureCollections iterates every active tenant and provisions
// collections for any enabled feature flags.
func ensureAllFeatureCollections(ctx context.Context, repos *postgresrepo.Repos, log *logger.Logger) error {
	active := true
	tenants, _, err := repos.Tenant.List(ctx, outbound.TenantFilter{Active: &active, Limit: 10_000})
	if err != nil {
		return fmt.Errorf("list tenants: %w", err)
	}

	for _, t := range tenants {
		t.Settings.NormalizePlugins()
		for _, id := range features.EnabledPlugins(t.Settings) {
			if err := features.EnsureFeatureCollections(ctx, t.ID,
				id, repos.Collection, repos.Field, repos.Section); err != nil {
				log.Errorw("bootstrap: ensure plugin collections",
					"tenant", t.ID, "plugin", id, "err", err)
			}
			if err := features.EnsureFeatureRelations(ctx, t.ID,
				id, repos.Relation); err != nil {
				log.Errorw("bootstrap: ensure plugin relations",
					"tenant", t.ID, "plugin", id, "err", err)
			}
		}
	}

	return nil
}
