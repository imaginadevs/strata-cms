# Relational items — API contract (any collection)

How to read and write related items over the item API. This is **generic**: it works
for any two collections connected by a declared relation, driven by relation metadata
— not tied to any one feature. For a concrete worked example see
[page-blocks-contract.md](page-blocks-contract.md); for the backend design see
[design-relational-io.md](design-relational-io.md).

Throughout, **P** is the parent collection and **C** its child collection, where C
holds a foreign-key column `fk` pointing at P (a one-to-many from P's side). In
relation metadata terms: `collection_many = C`, `field_many = fk`, `collection_one = P`.

## Relations are declared, not guessed

Expansion and nested writes only act on relations that exist in the tenant's
`relations` metadata. Feature-managed relations are provisioned automatically;
custom ones are created via the relations API. If no relation is declared between
P and C, the item API behaves as plain flat CRUD (backward compatible).

Each relation carries an `on_delete` that classifies it:
- **Owned (composition)** — `on_delete = cascade`. The child belongs to the parent:
  omitting it on save deletes it.
- **Referenced (association)** — `on_delete = restrict | set_null`. The child/target
  exists independently and is never deleted by the parent's save.

## Read

### Filtering & sorting (any list)
```
GET /api/items/{collection}?filter[<field>]=<value>&sort=<field>,-<other>&page=1&limit=25&lang=en
```
- `filter[field]=v` — equality; repeatable for multiple ANDed fields.
- `sort=a,-b` — ascending `a`, then descending `b` (`-` prefix = desc).
- Field names must be valid identifiers or the request is rejected (400).

### Expansion (`?deep=`)
Attach related children inline, one object instead of many round-trips:
```
GET /api/items/{P}/{id}?deep=<C>[,<C2>]&lang=en
GET /api/items/{P}?deep=<C>&filter[...]=...
```
```json
{
  "id": "parent-id", "…": "…",
  "<C>": [ { "id": "child-id", "<fk>": "parent-id", "…": "…" } ]
}
```
- Children are fetched in a single batched query (no N+1) and grouped per parent.
- Ordered by the child's `sort` column when it has one, else `created_at`.
- One level only (dotted `a.b` not yet supported). Unknown names are ignored.

## Write — nested reconcile (POST / PATCH)

Include a key named after the child collection **C** whose value is the desired array
of child rows. The parent's scalar fields and the children are written in **one
transaction**.

```
POST  /api/items/{P}            # create parent + children
PATCH /api/items/{P}/{id}       # update parent + reconcile children
```
```json
{
  "<parent field>": "…",
  "<C>": [
    { "id": "existing-child-id", "…": "…" },   // has id  → UPDATE
    { "…": "…" }                                // no id   → INSERT (fk set for you)
  ]
}
```

Reconcile rules, per child relation present in the body:
| Incoming | Action |
|----------|--------|
| row **with** `id` (must already belong to this parent) | UPDATE |
| row **without** `id` | INSERT, with `fk` forced to the parent id |
| existing child **omitted** from the array | DELETE if owned; left untouched if referenced |

Notes:
- The `fk` on every incoming child is forced to the parent — a client cannot reparent.
- Updating a child `id` that belongs to a **different** parent is rejected.
- **PATCH** only touches relations you include as keys; omit the key to leave that
  relation untouched. Only send a child array when you intend to reconcile it.
- The whole graph is transactional: any failure rolls the entire write back.

## Delete
- **Remove one child:** omit it from the array on PATCH (owned → row deleted), or
  `DELETE /api/items/{C}/{childId}` directly.
- **Delete the parent:** `DELETE /api/items/{P}/{id}`. Owned children are removed by
  the nested engine on save; note DB-level cascade (physical FK) is a follow-up, so
  deleting a parent does not yet auto-remove children outside a `SaveGraph` call.
- **Referenced targets** are never deleted through the parent.

## Current limits (see design doc)
Single-level `deep`; declarative-array writes only (no explicit `create/update/delete`
ops or pure-M2M id arrays yet); relation metadata is not backed by physical FKs;
filter equality compares on `::text` so it won't use an index yet; no GraphQL nesting.
