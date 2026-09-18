# Backlog

Deferred features and ideas not yet implemented. Each entry should capture *what*,
*why*, and enough *how* to pick up later without re-discovery.

---

## E-commerce tenant feature — IMPLEMENTED

**Status:** Done · **Implemented:** 2026-08-05

Ecommerce is implemented as the installable `ecommerce` plugin. Installation provisions
reserved product, variant, inventory, customer, method, order, transaction, and shipment
collections plus their relations and idempotent defaults. Tenant settings retain
`features.ecommerce` only as a compatibility mirror of `plugins.enabled.ecommerce`;
plugin entitlement and installation are authoritative.

Compiled generic and tenant-scoped tax, shipment, and payment calculators are available
through the quote API. See [ecommerce-calculators.md](ecommerce-calculators.md) for the
provider contract, built-ins, extension instructions, and security model.

---

> Deferred infra/consistency items (e.g. DB-side UUIDv7 defaults) live in
> [TECH_DEBT.md](TECH_DEBT.md), not here.

## Generic relational item I/O (nested read + nested write) â€” PHASES 1â€“2 DONE

**Status:** Prototype implemented â†’ [design-relational-io.md](design-relational-io.md)
(built+vetted, not yet DB-tested) Â· **Requested:** 2026-07-20 Â· **Priority:** high

Done: REST `filter[â€¦]`/`sort`; `?deep=` O2M expansion (batched); transactional nested
write (declarative array, owned-delete, reparent guard); Website relation metadata
auto-provisioned. Remaining: dotted `deep`, explicit-ops & pure-M2M writes, physical
FK/cascade, GraphQL nested resolvers, typed/indexed filtering, optional strict PUT,
and **DB integration tests** (use the `gen-repo-test` skill; assert tenant scoping +
owned-delete + reparent rejection).

Make relations a **generic** capability of the item layer, driven by `Relation`
metadata â€” not special-cased per collection (pages/blocks was the motivating case, but
this applies to *any* M2M/O2M). Today the item read/write path is entirely flat:
`item_repository` does single-table CRUD, GraphQL treats `relation` fields as scalars,
and relation metadata / junction tables are never consumed to nest or reconcile.

Target behaviour (Directus-style "single object view + relational save"):
- **Read:** opt-in expansion (`?deep` / `?fields=*,<rel>.*`) nests related rows into one
  object. Batch child fetches (`WHERE parent_id = ANY($1)`) â€” never N+1. Depth-capped.
- **Write:** accept the nested object; in ONE tenant-scoped transaction, upsert the
  parent, then per relation diff incoming children vs existing by `id` â†’
  insert / update / delete (+ `sort`). Validate every identifier with `safeIdent`.

Design notes:
- **Two M2M flavours, one engine.** Pure M2M (no payload, e.g. tagsâ†”posts) â†’ built-in
  junction, link/unlink ids. M2M *with payload* (pagesâ†”blocks: content, sort) â†’ model
  as an associative collection = parent **O2M** â†’ child, child **M2O** â†’ definition.
  The reconciler only needs O2M + M2O; pure M2M is the degenerate case.
- Supersedes the previously-proposed page-specific `PUT /pages/{id}/blocks` endpoint.

Phases: (1) one-level read expansion for O2M/M2O; (2) transactional nested write with
diff/reconcile; (3) M2M sugar (auto-junction) + depth/perf guards. Consider whether the
natural surface is REST, GraphQL (nested resolvers), or both.

---

## Blocks collection (Website feature) â€” IMPLEMENTED

**Status:** Done Â· **Implemented:** 2026-07-20

A reserved `blocks` collection is provisioned for tenants with the **Website** feature
enabled (`internal/core/features/features.go`, `websiteCollections()`), and the name
is reserved in `ReservedNames` so users cannot create their own `blocks` collection.

Fields (minimal by design â€” extend as needed):
- `title` â€” string, required.
- `key` â€” string, required. The **frontend component identifier**, stored **kebab-case**
  (e.g. `hero`, `featured-products`). Unique per `language` (system column), mirroring
  the `pages`/`posts` `{slug, language}` pattern, so a block can carry localized content.

**Key â†’ component convention:** the frontend resolves the stored kebab-case key to a
component deterministically, e.g. `featured-products` â†’ `FeaturedProducts.astro`
(kebab â†’ PascalCase file) or a camelCase import. Kebab-case was chosen over camelCase/
snake_case to stay consistent with the CMS `slug` convention and remain portable.

### Pages â†” Blocks (page-builder) â€” model IMPLEMENTED, wiring pending
The `pages_blocks` associative collection (many-to-many *with* payload) is now created
for Website tenants: `page_id`, `block_id`, `content` (jsonb), `sort`. Worked example in
[page-blocks-contract.md](page-blocks-contract.md); generic relational API contract in
[relational-items.md](relational-items.md).

Still to do for it to be usable end-to-end (see that doc's "Gaps" section):
- **Wire REST list filtering/sorting** (`filter[page_id]=`, `sort=`) â€” required for reads.
- Optional nested `PUT /api/pages/{id}/blocks` endpoint for atomic editor saves.
- Referential integrity (FK/relation on `page_id`/`block_id`) and a
  `pages_blocks(page_id, sort)` index.
- Decide whether `blocks.key` needs kebab-case validation at the API layer.
