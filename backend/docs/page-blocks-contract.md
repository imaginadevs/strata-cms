# Pages ↔ Blocks — worked example

How a **page** composes many **blocks**, where a *block* is a reusable component
**definition** and each placement carries **page-specific content**.

This is one concrete instance of the generic relational item API — for the full
read/write/delete contract (`?deep=`, `filter[]`, `sort`, nested POST/PATCH, owned vs
referenced delete) see **[relational-items.md](relational-items.md)**. Below covers
only what's specific to pages/blocks: the collections, the `key` → component
convention, and rendering.

## Model (many-to-many WITH payload)

Three reserved collections under the **Website** feature:

| Collection      | Role                                             |
|-----------------|--------------------------------------------------|
| `pages`         | The page (title, slug, status, …).               |
| `blocks`        | Block **definition** — `key` (component id) + `title`. Reusable across pages. |
| `pages_blocks`  | **Associative** row = one ordered block *instance* on a page. Holds the page-specific `content` + `sort`. |

`pages_blocks` is what makes this more than a plain M2M: the built-in junction table
only stores two FK columns, so it can't hold per-page content. A first-class
associative collection can.

### `blocks` (definition)
```json
{ "id": "uuid", "title": "Hero", "key": "hero", "language": "en" }
```
`key` is **kebab-case** and maps to a frontend component, e.g. `featured-products` →
`FeaturedProducts.astro`.

### `pages_blocks` (instance / placement)
```json
{
  "id": "uuid",
  "page_id": "uuid-of-page",
  "block_id": "uuid-of-block-definition",
  "content": { "heading": "Welcome", "cta": { "label": "Shop", "href": "/shop" } },
  "sort": 0,
  "language": "en"
}
```
- `content` is `jsonb` — free-form props for that placement.
- `sort` orders blocks within the page.
- A block key may repeat on a page (no unique on `page_id, block_id`).

## Composed shape to hand the frontend (for rendering)
```json
{
  "page": { "id": "…", "slug": "home", "title": "Home", "language": "en" },
  "blocks": [
    { "key": "hero",              "sort": 0, "content": { "heading": "Welcome" } },
    { "key": "featured-products", "sort": 1, "content": { "collection": "new-arrivals", "limit": 8 } }
  ]
}
```
Render loop (Astro example):
```astro
---
const map = { "hero": Hero, "featured-products": FeaturedProducts };
---
{page.blocks.map(b => { const C = map[b.key]; return C && <C {...b.content} /> })}
```

## TypeScript types
```ts
export interface BlockDefinition { id: string; key: string; title: string; language: string }
export interface PageBlock {
  id: string; page_id: string; block_id: string;
  content: Record<string, unknown>; sort: number; language: string;
}
export interface ComposedPage {
  page: { id: string; slug: string; title: string; language: string };
  blocks: Array<{ key: string; sort: number; content: Record<string, unknown> }>;
}
```

## API — this example mapped onto the generic contract

Full mechanics live in **[relational-items.md](relational-items.md)**. Here P = `pages`,
C = `pages_blocks` (fk `page_id`, owned/`cascade`); `pages_blocks` also references
`blocks` (fk `block_id`, referenced/`restrict`). All provisioned with the Website
feature.

- **Read one page + its blocks:** `GET /api/items/pages/{id}?deep=pages_blocks&lang=en`
  → `pages_blocks` attached as an array ordered by `sort`.
- **Map `block_id` → `key`:** `GET /api/items/blocks?lang=en` once (needed to pick the
  component; see below).
- **Save:** `POST /api/items/pages` (create) / `PATCH /api/items/pages/{id}` (update)
  with the desired `pages_blocks` array — reconciled in one transaction. Omitting an
  instance deletes it (owned); `blocks` definitions are never deleted this way.

## Block-specific: `key` → component
`blocks.key` is stored **kebab-case** and resolves to a frontend component
deterministically, e.g. `featured-products` → `FeaturedProducts.astro`. This mapping
is the only page-blocks-specific concern on top of the generic API — everything else
(deep/filter/sort/nested write/delete) is standard relational behaviour.
