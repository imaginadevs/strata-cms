package features

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// SeedEcommerceRBAC provisions the roles/permissions needed for a storefront:
//   - "Public" gets read access to the product catalog (no login required).
//   - "Ecommerce Customer" gets read on the catalog and read/create/update on
//     its own orders (row-level filter on customer_id).
//   - ecommerce_customers is wired as the auth collection for that role, so
//     POST /auth/ecommerce_customers/login issues tokens with it.
//
// Safe to call repeatedly (idempotent).
func SeedEcommerceRBAC(
	ctx context.Context,
	tenantID uuid.UUID,
	roleRepo outbound.RoleRepository,
	permRepo outbound.PermissionRepository,
	collRepo outbound.CollectionRepository,
) error {
	if roleRepo == nil || permRepo == nil {
		return nil
	}

	publicRole, err := ensureRole(ctx, tenantID, "Public", "Unauthenticated access", roleRepo)
	if err != nil {
		return fmt.Errorf("ecommerce rbac: %w", err)
	}
	for _, coll := range []string{"ecommerce_products", "ecommerce_product_categories"} {
		if err := upsertPermission(ctx, tenantID, publicRole.ID, coll, userdomain.ActionRead, nil, nil, permRepo); err != nil {
			return fmt.Errorf("ecommerce rbac: public read %s: %w", coll, err)
		}
	}

	customerRole, err := ensureRole(ctx, tenantID, "Ecommerce Customer", "Storefront customer", roleRepo)
	if err != nil {
		return fmt.Errorf("ecommerce rbac: %w", err)
	}
	for _, coll := range []string{"ecommerce_products", "ecommerce_product_categories"} {
		if err := upsertPermission(ctx, tenantID, customerRole.ID, coll, userdomain.ActionRead, nil, nil, permRepo); err != nil {
			return fmt.Errorf("ecommerce rbac: customer read %s: %w", coll, err)
		}
	}
	ownOrders := map[string]any{"customer_id": map[string]any{"_eq": "$CURRENT_USER"}}
	for _, action := range []userdomain.Action{userdomain.ActionRead, userdomain.ActionCreate, userdomain.ActionUpdate} {
		if err := upsertPermission(ctx, tenantID, customerRole.ID, "ecommerce_orders", action, nil, ownOrders, permRepo); err != nil {
			return fmt.Errorf("ecommerce rbac: customer orders %s: %w", action, err)
		}
	}

	if collRepo != nil {
		if err := setAuthDefaultRole(ctx, tenantID, "ecommerce_customers", customerRole.Name, collRepo); err != nil {
			return fmt.Errorf("ecommerce rbac: %w", err)
		}
	}
	return nil
}
