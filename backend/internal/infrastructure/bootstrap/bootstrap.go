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

	// 0. Make sure a platform tenant exists — it owns the super-admin account.
	platform := ensurePlatformTenant(ctx, repos, cfg, log)

	// 1. Per-tenant schema migrations + default (non-super) admin seed. For every
	//    tenant that is NOT the platform tenant, clear any stray super-admin flag
	//    so only the platform tenant holds super admins.
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
		if platform == nil || t.ID != platform.ID {
			if err := migrator.DemoteSuperAdmins(ctx, t.ID); err != nil {
				log.Errorw("bootstrap: demote super admins failed", "tenant", t.ID, "err", err)
			}
		}
	}

	// 2. Seed the platform super-admin from env into the platform tenant schema.
	if platform != nil {
		if err := migrator.EnsureSchema(ctx, platform.ID); err != nil {
			log.Errorw("bootstrap: platform schema migration failed", "tenant", platform.ID, "err", err)
		} else {
			_ = migrator.SeedDefaults(ctx, platform.ID, adminEmailFor(platform))
			if err := migrator.SeedSuperAdmin(ctx, platform.ID, cfg.SuperAdmin.Email, cfg.SuperAdmin.Password); err != nil {
				log.Errorw("bootstrap: seed platform super admin failed", "tenant", platform.ID, "err", err)
			} else if cfg.SuperAdmin.Email != "" && cfg.SuperAdmin.Password != "" {
				log.Infow("bootstrap: platform super admin ensured", "tenant", platform.ID, "email", cfg.SuperAdmin.Email)
			} else {
				log.Warnw("bootstrap: SUPER_ADMIN_EMAIL/PASSWORD not set — no super admin seeded; tenant management will be unavailable")
			}
		}
	} else {
		log.Warnw("bootstrap: no platform tenant resolved — super admin not seeded")
	}

	// 3. Feature collections.
	if err := ensureAllFeatureCollections(ctx, repos, log); err != nil {
		log.Errorw("bootstrap: feature collection provisioning had errors", "err", err)
	}

	return nil
}

// ensurePlatformTenant returns the single platform tenant, creating or flagging
// one when necessary:
//   - if one is already flagged, use it;
//   - else flag the bootstrap 'default' tenant;
//   - else create a dedicated 'platform' tenant.
//
// When PLATFORM_DOMAIN is configured it is applied to the platform tenant's
// domain so the super-admin can sign in at that host. Returns nil only if every
// step failed (logged).
func ensurePlatformTenant(ctx context.Context, repos *postgresrepo.Repos, cfg *config.Config, log *logger.Logger) *tenantdomain.Tenant {
	pt, err := repos.Tenant.GetPlatform(ctx)
	if err != nil {
		log.Errorw("bootstrap: get platform tenant failed", "err", err)
	}

	if pt == nil {
		// Prefer the bootstrap 'default' tenant as the platform tenant.
		cand, _ := repos.Tenant.GetBySlug(ctx, "default")
		if cand == nil {
			// No default tenant: create a dedicated platform tenant.
			nt := tenantdomain.New("Platform", "platform", "en")
			nt.IsPlatform = true
			if cfg.SuperAdmin.PlatformDomain != "" {
				d := cfg.SuperAdmin.PlatformDomain
				nt.Domain = &d
			}
			if err := repos.Tenant.Create(ctx, nt); err != nil {
				log.Errorw("bootstrap: create platform tenant failed", "err", err)
				return nil
			}
			log.Infow("bootstrap: created platform tenant", "tenant", nt.ID, "slug", nt.Slug)
			return nt
		}
		if err := repos.Tenant.SetPlatform(ctx, cand.ID); err != nil {
			log.Errorw("bootstrap: flag platform tenant failed", "tenant", cand.ID, "err", err)
			return nil
		}
		cand.IsPlatform = true
		pt = cand
		log.Infow("bootstrap: flagged platform tenant", "tenant", pt.ID, "slug", pt.Slug)
	}

	// Apply the configured platform domain so the operator can sign in there.
	if cfg.SuperAdmin.PlatformDomain != "" && (pt.Domain == nil || *pt.Domain != cfg.SuperAdmin.PlatformDomain) {
		d := cfg.SuperAdmin.PlatformDomain
		pt.Domain = &d
		if err := repos.Tenant.Update(ctx, pt); err != nil {
			log.Errorw("bootstrap: set platform domain failed", "tenant", pt.ID, "domain", d, "err", err)
		}
	}
	return pt
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
			if err := features.EnsureFeatureWorkflows(ctx, t.ID,
				id, repos.Workflow); err != nil {
				log.Errorw("bootstrap: ensure plugin workflows",
					"tenant", t.ID, "plugin", id, "err", err)
			}
		}
	}

	return nil
}
