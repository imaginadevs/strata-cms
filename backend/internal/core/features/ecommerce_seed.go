package features

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// SeedEcommerceDefaults creates the built-in location and calculator methods.
// It is safe to call repeatedly.
func SeedEcommerceDefaults(ctx context.Context, tenantID uuid.UUID, items outbound.ItemRepository) error {
	defaults := []struct {
		collection string
		code       string
		data       map[string]any
	}{
		{"ecommerce_locations", "default", map[string]any{"name": "Default", "code": "default", "active": true}},
		{"ecommerce_payment_methods", "manual", map[string]any{"name": "Manual", "code": "manual", "provider": "generic.payment.manual", "active": true, "config": map[string]any{}, "sort": 0}},
		{"ecommerce_payment_methods", "wompi", map[string]any{"name": "Wompi", "code": "wompi", "provider": "wompi", "active": false, "config": map[string]any{"environment": "sandbox"}, "sort": 1}},
		{"ecommerce_shipment_methods", "flat", map[string]any{"name": "Flat Rate", "code": "flat", "provider": "generic.shipment.flat_rate", "active": true, "config": map[string]any{"flat": 0}, "sort": 0}},
		{"ecommerce_tax_methods", "none", map[string]any{"name": "No Tax", "code": "none", "provider": "generic.tax.none", "active": true, "config": map[string]any{}, "sort": 0}},
	}
	for _, seed := range defaults {
		rows, _, err := items.List(ctx, tenantID, seed.collection, outbound.QueryFilter{Filter: map[string]any{"code": seed.code}, Limit: 1})
		if err != nil {
			return fmt.Errorf("seed ecommerce %s: list code %q: %w", seed.collection, seed.code, err)
		}
		if len(rows) != 0 {
			continue
		}
		if _, err := items.Create(ctx, tenantID, seed.collection, seed.data); err != nil {
			return fmt.Errorf("seed ecommerce %s code %q: %w", seed.collection, seed.code, err)
		}
	}
	return nil
}
