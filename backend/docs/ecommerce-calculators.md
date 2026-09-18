# Ecommerce calculators

The ecommerce plugin calculates tax, shipping, and payment fees through compiled Go providers. Provider IDs are stored in the `provider` field of the corresponding ecommerce method row. Configuration is JSON in that row's `config` field.

## Provider scopes

A provider registered with a `nil` tenant scope is generic and is available to every tenant. A provider registered with a tenant UUID is available only when the quote's authoritative tenant ID matches that UUID. If a tenant-scoped provider and a generic provider have the same ID, the tenant-scoped implementation wins for that tenant.

Provider lookup never uses a tenant ID supplied in the request body. The REST handler overwrites `OrderDraft.tenant_id` with the tenant resolved by authentication and domain middleware.

## Security model

Calculators are compiled into the server binary. The registry does not evaluate Go, JavaScript, templates, expressions, or strings supplied by a tenant. Method configuration is data only and must be validated by each calculator. Never add `eval` or a dynamic script loader.

## Add a tenant calculator

1. Add a Go file under `internal/core/ecommerce/tenants/`.
2. Implement `ecommerce.CalculatorFunc`.
3. Register it from `init` with the fixed tenant UUID and a stable provider ID.
4. Set the relevant payment, shipment, or tax method row's `provider` field to that ID.

```go
package tenants

import (
    "context"
    "encoding/json"

    "github.com/google/uuid"
    "github.com/imaginacolombia/cms/internal/core/ecommerce"
)

var tenantID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

func init() {
    ecommerce.Register(ecommerce.KindShipment, "tenant.acme.shipment.local", &tenantID,
        func(ctx context.Context, draft ecommerce.OrderDraft, config json.RawMessage) (ecommerce.CalcResult, error) {
            return ecommerce.CalcResult{Amount: 5}, nil
        })
}
```

The tenants package is blank-imported from `internal/adapters/http/router.go` so its `init` functions run. Keep that import when adding tenant files. Keep provider code deterministic, context-aware, and free of network calls unless timeouts and failure behavior are explicit.

## OrderDraft input

`OrderDraft` includes currency, lines (`quantity` and `unit_price`), customer details, shipping and billing addresses, tax country, and method references. Each method reference contains a `provider` ID and JSON `config`. During quoting, the core computes subtotal first, then tax, shipping, and payment fee. A payment surcharge therefore sees the computed subtotal, shipping total, and tax result.

When tax returns `declare_external`, `tax_total` remains visible in the quote but is excluded from `total` because the payment provider is expected to declare or collect it externally.

## Built-in providers

- `generic.payment.manual`: zero payment fee.
- `generic.payment.surcharge_pct`: `config.percent` of subtotal + shipping + charged tax.
- `generic.shipment.flat_rate`: fixed `config.flat` amount.
- `generic.tax.none`: zero tax with `charge` mode.
- `generic.tax.rate_by_country`: `config.rates` maps country codes to percentage rates applied to subtotal. It returns `declare_external` when payment config has `declare_tax_with_provider: true`.

Use `GET /api/ecommerce/providers?kind=payment|shipment|tax` to discover providers available to the authenticated tenant and `POST /api/ecommerce/quote` to calculate totals.
