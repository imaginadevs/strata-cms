# Design: Generic relational item I/O (nested read + nested write)

**Status:** Phases 1–2 IMPLEMENTED (prototype on pages/pages_blocks/blocks) ·
**Created:** 2026-07-20

> Implemented so far: REST `filter[…]`/`sort` on list; `?deep=` one-level O2M
> expansion (batched, no N+1); transactional nested write (declarative array,
> insert/update/delete with owned-delete + within-tenant reparent guard); Website
> relation metadata auto-provisioned. Code: `item_graph.go`, `item_repository.go`
> (List), `rest/items.go`, `features` (RelationsForFeature / EnsureFeatureRelations).
> Not yet: dotted deep, explicit-ops/pure-M2M writes, physical FKs, GraphQL nesting,
> typed (indexed) filtering, strict PUT. **Verified by build+vet; not yet DB-tested.**
>
> **Studio (cms/frontend):** the Content editor now supports item + subitems
> generically, driven by relation metadata. `lib/relational.ts` discovers child
> (O2M) relations and reads via `?deep=`; `components/content/SubitemsEditor.tsx`
> is a dnd-kit sortable child-row editor using the shared `components/content/
> fields.tsx` renderers; `pages/Content.tsx` loads the item expanded, renders one
> editor per child relation, and saves the whole graph in a single nested write.
> Type-checks clean. (Studio `npm run lint` is broken repo-wide — ESLint 9 with a
> legacy config, pre-existing and unrelated.)

Make nested reads and relational saves a **generic** capability of the item layer,
driven by existing `Relation` metadata — so *any* collection with relations gets a
"single object view" on read and an atomic "save the object and its relations" on
write. Motivating case: `pages` ↔ `pages_blocks` ↔ `blocks`, but nothing here is
page-specific.

Today (baseline): `item_repository` is flat single-table CRUD; GraphQL resolves
`relation` fields as scalars; junction tables and `Relation` metadata are never
consumed for nesting or reconciliation.

---

## 1. Concepts

### Relation shapes (all reduce to O2M + M2O)
- **many-to-one (M2O):** child holds a FK column → parent (`pages_blocks.page_id`).
- **one-to-many (O2M):** inverse of the above (`pages.pages_blocks`).
- **many-to-many (M2M):** two O2M/M2O through a link table.
  - *Pure M2M* (no payload, e.g. `tags`↔`posts`): built-in junction, link/unlink ids.
  - *M2M with payload* (e.g. `pages`↔`blocks` carrying `content`,`sort`): modeled as an
    **associative collection** (`pages_blocks`) = parent **O2M** → child, child **M2O** →
    definition. The reconciler only needs O2M + M2O.

### Owned vs referenced (drives delete semantics)
- **Owned (composition):** child rows belong to the parent. Removing from the payload
  **deletes** the row; deleting the parent **cascades**. (`pages_blocks` is owned by
  `pages`.) Convention: an O2M whose relation `on_delete = cascade`.
- **Referenced (association):** the related entity exists independently. Removing from
  the payload only **unlinks** (deletes the junction row / nulls the FK); the entity
  survives. (`blocks` definitions; `tags`.) Convention: `on_delete = set null|restrict`.

The engine reads this from `Relation.OnDelete` (already on the entity). Recommend adding
an explicit `Relation.Owned bool` later to avoid overloading `on_delete`.

---

## 2. Read — expansion

Opt-in only (keeps list endpoints lean — a project performance requirement).

```
GET /api/items/pages/{id}?deep=pages_blocks
GET /api/items/pages?filter[slug]=home&deep=pages_blocks.block_id&lang=en
```

- `deep=relA,relB` expands one or more relations; dotted path expands nested
  (`pages_blocks.block_id` also inlines each block definition).
- **Depth cap** (default 2, hard max 3). **Child-count cap** per relation (e.g. 500).
- **No N+1:** resolve one relation for the whole result page in a single query
  `... WHERE <fk> = ANY($1)` then group in memory. Never per-parent queries.
- Ordering: children come back ordered by their collection's natural sort; for
  `pages_blocks` order by `sort ASC` (engine applies the child collection's default sort
  if declared, else `sort` then `created_at`).

Expanded response:
```json
{
  "id": "p1", "title": "Home", "slug": "home", "language": "en",
  "pages_blocks": [
    { "id": "pb1", "block_id": "b_hero",     "sort": 0, "content": { "heading": "Welcome" } },
    { "id": "pb2", "block_id": "b_featured", "sort": 1, "content": { "limit": 8 } }
  ]
}
```

---

## 3. Write — nested reconcile (POST / PATCH)

The payload is the **desired state**. Scalar fields are written to the parent row;
any key that matches a relation name is reconciled.

### Reconcile algorithm (per relation, inside ONE transaction)
```
1. upsert parent scalar fields (INSERT on POST, UPDATE on PATCH), get parentID.
2. for each relation key present in the body:
   a. load existing child ids for parentID (tenant-scoped).
   b. incoming child WITH id     -> UPDATE  (assert row belongs to parentID+tenant).
   c. incoming child WITHOUT id   -> INSERT  (set FK = parentID).
   d. existing id NOT in incoming -> DELETE  (owned) | UNLINK (referenced).
   e. apply `sort` from array order or explicit `sort` field.
3. commit. Any error -> rollback the whole graph.
```

### Two body forms
**Declarative array (default)** — send the full desired child set; omissions are
deletes/unlinks. Natural for a page-builder "save":
```json
{
  "title": "Home v2",
  "pages_blocks": [
    { "id": "pb1", "sort": 0, "content": { "heading": "Welcome back" } },
    { "block_id": "b_cta", "sort": 1, "content": { "label": "Shop" } }
  ]
}
```

**Explicit ops (opt-in, safer for large/partial edits)** — no accidental mass-delete:
```json
{
  "pages_blocks": {
    "create": [{ "block_id": "b_cta", "sort": 2, "content": {} }],
    "update": [{ "id": "pb1", "sort": 0 }],
    "delete": ["pb2"]
  }
}
```

**Pure M2M** uses an id array (link/unlink to match the set):
```json
{ "tags": ["t_go", "t_cms"] }
```

### PATCH vs PUT semantics (recommended)
- **POST** `/items/{collection}` — create parent + nested children (children have no id).
- **PATCH** `/items/{collection}/{id}` — **present-key reconcile**: only scalar fields and
  relation keys *actually in the body* are touched; absent relations are left untouched.
  This is the safe default and matches the existing route.
- **PUT** `/items/{collection}/{id}` — optional strict full-replace alias: relations
  absent from the body are treated as empty (delete/unlink all). Powerful but a foot-gun;
  add only if a client wants true idempotent replace, and document it loudly.

> The editor "Save" button should use **PATCH with the declarative array** for the
> relations it manages. That already gives create/update/delete via one call without the
> risk of PUT wiping relations the client didn't send.

---

## 4. Delete

- **Remove a child from a page:** omit it from the `pages_blocks` array on PATCH (or use
  `delete:[id]`). Owned → row deleted. (Referenced/M2M → only the link is removed.)
- **Delete the page:** `DELETE /api/items/pages/{id}` cascades to owned children
  (`pages_blocks`). Requires real FKs with `ON DELETE CASCADE`, or the engine deletes
  owned children in the same transaction. Referenced definitions (`blocks`) are never
  touched.
- **Delete a block definition in use:** blocked (`on_delete = restrict`) or nulled,
  never silently cascaded into pages.

---

## 5. Cross-cutting requirements
- **Tenant isolation:** every statement fully-qualified to `schemaOf(tenantID)`; every
  child asserted to belong to the parent **and** tenant before update/delete.
- **Injection safety:** all identifiers (collection, column, junction, relation fields)
  through `safeIdent`; all values as pgx bind params.
- **Atomicity:** the whole graph writes in one `pgx.Tx`; partial failure rolls back.
- **Validation:** unknown relation keys → 422; per-child errors returned keyed by index
  or `id`. Field validation reuses the collection's field rules.
- **Performance:** expansion opt-in + depth/count caps; batched child fetch (`= ANY`);
  index owned children on `(<fk>, sort)`.
- **Events:** emit existing item events per affected row (or a coalesced graph event) so
  SSE/Kafka stay correct.

---

## 6. Phases
1. **Read expansion** (O2M/M2O, one level, `?deep=`) + wire `filter`/`sort` into the REST
   list handler. Independently useful; unblocks the frontend single-object view.
2. **Nested write reconcile** (POST + PATCH, declarative array + explicit ops) in a tx.
3. **M2M sugar** (auto-junction link/unlink, id arrays), depth/count guards, GraphQL
   nested resolvers, optional strict `PUT`.

---

## 7. Prototype target
`pages` + `pages_blocks` + `blocks`: expand on read, reconcile on write, delete-by-omit.
Proves O2M + M2O + owned-delete before generalizing to all collections.
