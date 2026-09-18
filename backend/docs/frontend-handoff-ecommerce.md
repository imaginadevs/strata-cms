# Frontend handoff — backend changes (2026-09-08)

Backend changes that (optionally) touch the frontend. **Nothing here is required for
existing screens to keep working** — the studio is schema-driven and never reads
`tenant_id`. These are the tidy-ups and new capabilities.

Two audiences:
- **Admin studio** = `cms/frontend` (React) → tasks 1 & 2.
- **Storefront** = the per-tenant delivery site (`frontend-*`) → task 3.

All endpoints are under `/api`. Auth: `Authorization: Bearer <access_token>` for
admin calls; storefront checkout is cart-scoped (see task 3).

---

## Task 1 — Remove `tenant_id` from TS types (admin studio, 2 min)

The backend dropped the `tenant_id` column everywhere (tenancy is enforced by the
DB schema). The API no longer returns `tenant_id` on any entity. Nothing reads it,
so this is just keeping types honest.

In `cms/frontend/src/types/index.ts`, delete the `tenant_id` property from these
interfaces (search `tenant_id` — 5 hits):
- `BaseEntity`
- `Relation`
- `CollectionIndex`
- `AuditLog`
- the remaining optional one (`tenant_id?: string`)

No other change needed. `src/components/content/fields.tsx` may keep `"tenant_id"`
in its `systemKeys` skip-set — harmless, leave or remove.

---

## Task 2 — Ecommerce admin config (provisioned by plugin activation — NOT a frontend build)

**This is not frontend work.** Both the inventory settings and the Wompi payment
method are provisioned by the backend when the **ecommerce plugin is activated**
(`POST /api/plugins/ecommerce/install`), and both are edited through screens the
studio *already* has (the generic settings editor and the generic collection/item
editor). No bespoke ecommerce forms are required. This section documents what
activation sets up and which existing screens edit it.

### 2a. Inventory settings — provisioned on first activation

On the **first** ecommerce activation the backend writes conservative defaults into
the tenant settings (idempotent re-install never overwrites operator changes):
```jsonc
// GET /api/settings  → { tenant, settings }
settings.ecommerce = {
  "track_inventory": false,  // checkout validates + decrements stock when true
  "allow_backorder": false   // (only when track_inventory) allow stock to go negative
}
```
These are edited like any other tenant setting via the existing settings screen.
`PATCH /api/settings` *replaces* the whole `settings` object, so read-modify-write
(send the full `settings` back with only `ecommerce` changed):
```
GET /api/settings                     → settings
PATCH /api/settings  { "settings": { ...settings, "ecommerce": { track_inventory, allow_backorder } } }
```
If the studio's settings screen is hand-built and doesn't yet surface these two
booleans, exposing them there is a *tiny* addition to an existing form — not a new
feature. The values, their meaning, and their defaults are all owned by the plugin.

### 2b. Wompi payment method — seeded on activation, edited via the generic item editor

Activation seeds two rows in `ecommerce_payment_methods`: `manual` (active) and
`wompi` (inactive, `config: { environment: "sandbox" }`). The wompi row is edited
through the **generic collection/item editor the studio already renders** — its
`config` (JSONB) and `active` fields, no custom form:
```
PATCH /api/items/ecommerce_payment_methods/{id}
{
  "active": true,
  "config": {
    "environment": "sandbox",          // "sandbox" | "production"
    "public_key": "pub_test_xxx",
    "private_key": "prv_test_xxx",      // optional (server API calls)
    "integrity_secret": "test_integrity_xxx",   // required — signs the checkout
    "events_secret": "test_events_xxx",          // required — verifies webhooks
    "redirect_url": "https://store.example.com/checkout/return"  // optional default
  }
}
```
An optional pre-save **validation** endpoint exists (backend) if you want to check
credentials before flipping `active` — it is not required to make editing work:
```
POST /api/ecommerce/payment-methods/validate
  { "payment_method_id": "<id>" }          // validate the stored config
  — or —
  { "provider": "wompi", "config": { ... } } // validate an inline config
→ { "valid": true }  |  { "valid": false, "error": "wompi: integrity_secret is required" }
```
Requires the `settings:update` permission and the ecommerce plugin installed.

> Note: `carts` and `cart_items` collections are also created by activation and
> appear automatically in the schema-driven collection/content UI — no work needed.

---

## Task 3 — Storefront cart + checkout (delivery frontend)

New capability. There are **no dedicated cart-mutation routes** — carts are managed
through the generic items API; only `checkout` is a custom endpoint. Guest carts use
a client-generated `token`; logged-in customers can use `customer_id`.

### Catalog (read)
```
GET /api/items/ecommerce_products?filter[status]=published&page=1&limit=24
GET /api/items/ecommerce_products/{id}?deep=ecommerce_product_variants
GET /api/items/ecommerce_product_categories
```

### Cart lifecycle (generic items API)
```
// 1. Create a cart (guest: generate a token client-side and keep it in storage)
POST /api/items/ecommerce_carts
  { "token": "<random>", "currency": "COP", "status": "active", "email": "..." }
→ { id, token, ... }

// 2. Add / update / remove lines
POST   /api/items/ecommerce_cart_items
  { "cart_id": "<cartId>", "product_id": "<id>", "variant_id": "<id|null>",
    "title": "...", "sku": "...", "quantity": 2, "unit_price": 50000 }
PATCH  /api/items/ecommerce_cart_items/{id}   { "quantity": 3 }
DELETE /api/items/ecommerce_cart_items/{id}

// 3. Read the cart with its lines nested (relation is registered → ?deep works)
GET /api/items/ecommerce_carts/{id}?deep=ecommerce_cart_items
```
`unit_price`/`title` on the cart line are for display only — **checkout re-resolves
prices from the product/variant server-side**, so the client can't tamper with them.

### Checkout
```
POST /api/ecommerce/checkout
{
  "cart_id": "<cartId>",           // or "cart_token": "<token>"
  "payment_method_id": "<id>",     // required (manual or wompi)
  "shipment_method_id": "<id>",    // optional
  "tax_method_id": "<id>",         // optional
  "tax_country_code": "CO",
  "customer": { "email": "a@b.co", "name": "...", "phone": "...",
                "identification_type": "CC", "identification_number": "123",
                "id": "<ecommerce_customers id, optional>" },
  "shipping_address": { ... }, "billing_address": { ... },
  "redirect_url": "https://store.example.com/checkout/return"
}
→ 201 {
  "data": {
    "order":   { "id", "order_number", "status": "pending", "total", "currency", ... },
    "payment": {
      "status": "pending",
      "provider_reference": "",
      "redirect_url": "https://checkout.wompi.co/p/?...",  // present for wompi
      "checkout": { public_key, currency, amount_in_cents, reference, signature_integrity, ... }
    }
  }
}
```
Handle the response by payment method:
- **manual** → `payment.status = "pending"`, no `redirect_url`. Show an order-received
  page; payment is reconciled offline by an admin.
- **wompi** → **redirect the browser to `payment.redirect_url`** (or use the returned
  `checkout` params with the Wompi Web Checkout widget). The buyer pays on Wompi and
  returns to your `redirect_url`.

Error cases: `409 {"error":"insufficient stock"}` (when `track_inventory` on and no
backorder), `400`/`409` for empty/converted cart, or `{"data":{"order":..,"payment_error":".."}}`
if the gateway call failed after the order was created.

### After the redirect (async payment)
The **authoritative** result arrives asynchronously via a Wompi → backend webhook
(`POST /api/ecommerce/webhooks/wompi` — **backend only, do not call from the client**),
which flips the order to `paid`/`awaiting_payment`. On your return page, **poll the
order** until its status settles:
```
GET /api/items/ecommerce_orders/{orderId}
→ status: pending → paid | awaiting_payment | cancelled
```
Don't trust the redirect alone for "paid" — confirm via the order status.

### Customer auth (optional)
`ecommerce_customers` is an auth collection, so customers can have accounts (the
session/auth system is principal-based). Guest checkout via `cart_token` needs no
login. Wire customer login only if you want accounts/order history.

---

## Not required (transparent backend changes)
Audit logging, the Last Login fix, logout/change-password fixes, and JSON error
bodies keep the same request/response shapes — no frontend change.

## Backend caveats the frontend should know
- Checkout is **not yet fully atomic** (order+items are; the transaction/stock/cart
  writes are separate) and inventory uses `products.stock` / `variants.stock` (not the
  multi-location `inventory_levels`). Behaviour is correct for the happy path; edge
  cases may need reconciliation. Track on the backend side.
- New tenants get the cart collections + wompi method automatically; existing tenants
  need a backend restart to provision them.
