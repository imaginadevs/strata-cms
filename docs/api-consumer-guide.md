# Consuming the CMS API — REST & GraphQL

A practical guide for a developer who wants to read and write content through the
CMS. It covers authentication, discovering your schema, and both the **REST** and
**GraphQL** interfaces — including filtering, sorting, pagination, relations and
languages.

> Replace `cms.imaginadevs.com` with your tenant's domain and `blog_posts` with
> one of your collections throughout.

---

## 1. Core concepts

- **Everything is a collection.** Your content models (`blog_posts`, `products`,
  `pages`, …) are defined at runtime. Each row is an *item*.
- **Multi-tenant by domain.** The tenant is resolved from the **Host** of the
  request — you reach a tenant by calling *its* domain. There is no tenant header
  or query param.
- **Two APIs over the same data:**
  - **REST** — best for precise **field filtering, sorting, relations (`deep`) and
    language**.
  - **GraphQL** — best for **shaping the exact fields you want** and **keyword
    search**; the schema is generated automatically from your collections.
- **Base URL:** `https://cms.imaginadevs.com/api`

---

## 2. Authentication

Auth is **JWT token-based**. Log in, hold the returned access token, and send it as
`Authorization: Bearer <access_token>` on every call. Item endpoints use *optional*
auth: a collection with a **Public read** permission can be read without a token;
everything else needs a valid token and the right role permission.

### Log in (returns the tokens)

```bash
curl -s -X POST https://cms.imaginadevs.com/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"••••••"}'
```
```json
{
  "access_token":  "eyJhbGci…",     // short-lived JWT — send as Bearer
  "refresh_token": "e433d745…",     // opaque — exchange for a new access token
  "expires_at":    "2026-01-01T00:00:00Z",
  "user": { "id": "…", "email": "…", "roles": ["Administrator"], "is_super_admin": false }
}
```

### Use the token on every subsequent call

```bash
TOKEN="eyJhbGci…"
curl -s https://cms.imaginadevs.com/api/items/blog_posts \
  -H "Authorization: Bearer $TOKEN"
```

### Keeping the session alive

```bash
# When a call returns 401 (access token expired), exchange the refresh token:
curl -s -X POST https://cms.imaginadevs.com/api/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"e433d745…"}'
# → a fresh { access_token, refresh_token, expires_at, user }
```

Notes:
- `GET /api/auth/me` returns the current user for a token (or `401`).
- `POST /api/auth/logout` `{ "refresh_token": "…" }` revokes the server-side session.
- **Multi-tenant:** the token embeds its tenant, and the tenant is resolved from the
  request **Host** — so call the token's own tenant domain. A **super-admin** token
  may target another tenant by adding an `X-Tenant-ID: <tenant-uuid>` header.
- (The server also accepts the JWT via an `access_token` cookie for browser clients,
  but Bearer is the primary, uniform model.)

---

## 3. Discover your schema

Before querying, list your collections and their fields.

```bash
# All collections
curl -s -H "Authorization: Bearer $TOKEN" https://cms.imaginadevs.com/api/collections

# One collection with its fields (name, type, required, …)
curl -s -H "Authorization: Bearer $TOKEN" https://cms.imaginadevs.com/api/collections/blog_posts
```

Field types you'll see: `string, text, rich_text, integer, float, decimal,
boolean, date, datetime, time, json, uuid, relation, file, files, select,
multiselect, hash, ip, timestamp, csv, geometry`.

Every item also has the **system fields**: `id`, `created_at`, `updated_at`,
`created_by`, `updated_by`.

---

## 4. REST API

Base path: `/api/items/{collection}`

| Method & path | Purpose |
|---|---|
| `GET  /api/items/{collection}` | List items (filter, sort, paginate, expand) |
| `GET  /api/items/{collection}/{id}` | Get one item |
| `POST /api/items/{collection}` | Create an item |
| `PATCH /api/items/{collection}/{id}` | Update an item (partial) |
| `DELETE /api/items/{collection}/{id}` | Delete an item |

### 4.1 List response envelope

```json
{
  "data":  [ { "id": "…", "title": "…", "status": "published", "…": "…" } ],
  "total": 128,
  "page":  1,
  "limit": 50
}
```

### 4.2 Pagination

- `page` — 1-based (default `1`).
- `limit` — items per page, `1`–`200` (default `50`; out-of-range falls back to 50).

```bash
curl -s -H "Authorization: Bearer $TOKEN" "https://cms.imaginadevs.com/api/items/blog_posts?page=2&limit=20"
```

### 4.3 Filtering — `filter[field]=value`

Filtering is **field equality**. Each `filter[...]` is ANDed together.

```bash
# status == "published"
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://cms.imaginadevs.com/api/items/blog_posts?filter[status]=published"

# status == "published" AND author_id == <uuid>
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://cms.imaginadevs.com/api/items/blog_posts?filter[status]=published&filter[author_id]=8f3c…"
```

- The field name must be a valid identifier (letters, digits, underscore) — an
  invalid name returns `400`.
- **Only equality is supported** for client filters (no `>=`, `<`, `like`, `in`).
  For ranges/full-text, use GraphQL `search` or filter client-side. (Range
  operators exist internally only for row-level permission rules, not for callers.)

### 4.4 Sorting — `?sort=`

Comma-separated field list; a leading `-` means descending.

```bash
# newest first, then title A→Z
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://cms.imaginadevs.com/api/items/blog_posts?sort=-created_at,title"
```

### 4.5 Relations — `?deep=`

Expand **one-to-many children** inline (e.g. a page and its blocks). Give the
child collection name(s), comma-separated.

```bash
# each page with its pages_blocks children attached
curl -s -H "Authorization: Bearer $TOKEN" \
  "https://cms.imaginadevs.com/api/items/pages/PAGE_ID?deep=pages_blocks"
```

`deep` works on both list and single-item GET. A `relation` field itself is just
the related row's `id` — use `deep` to pull the child rows in.

### 4.6 Language — `?lang=`

For translatable content, request a specific language:

```bash
curl -s -H "Authorization: Bearer $TOKEN" "https://cms.imaginadevs.com/api/items/blog_posts?lang=es"
```

### 4.7 Create

Send a JSON object of field values. The response is the created item.

```bash
curl -s -H "Authorization: Bearer $TOKEN" -X POST https://cms.imaginadevs.com/api/items/blog_posts \
  -H 'Content-Type: application/json' \
  -d '{"title":"Hello","status":"draft","body":"..."}'
```

**Nested write:** include child rows under the child-collection key to create the
whole graph in one request:

```bash
curl -s -H "Authorization: Bearer $TOKEN" -X POST https://cms.imaginadevs.com/api/items/pages \
  -H 'Content-Type: application/json' \
  -d '{
        "title": "Landing",
        "pages_blocks": [
          {"type": "hero", "sort": 0},
          {"type": "text", "sort": 1}
        ]
      }'
```

### 4.8 Update / Delete

```bash
# Partial update — send only the fields you change
curl -s -H "Authorization: Bearer $TOKEN" -X PATCH https://cms.imaginadevs.com/api/items/blog_posts/ITEM_ID \
  -H 'Content-Type: application/json' \
  -d '{"status":"published"}'

# Delete
curl -s -H "Authorization: Bearer $TOKEN" -X DELETE https://cms.imaginadevs.com/api/items/blog_posts/ITEM_ID
```

### 4.9 Full example — filter + sort + paginate

```bash
curl -s -H "Authorization: Bearer $TOKEN" "https://cms.imaginadevs.com/api/items/blog_posts\
?filter[status]=published&sort=-created_at&page=1&limit=10&lang=en"
```

---

## 5. GraphQL API

- **Endpoint:** `POST https://cms.imaginadevs.com/api/graphql`
- **Playground:** open `https://cms.imaginadevs.com/api/graphql` in a browser
  (GraphiQL — introspection enabled).
- The schema is **generated per tenant** from your collections and refreshes
  automatically when you change the schema.

### 5.1 Naming conventions

For a collection `blog_posts`:

| Thing | Name |
|---|---|
| List query | `blog_posts(limit, page, search)` |
| Single query | `blog_postsByID(id)` |
| Object type | `BlogPostsType` |
| Input type | `BlogPostsInput` |
| Create / Update / Delete | `createBlogPosts`, `updateBlogPosts`, `deleteBlogPosts` |

(Query field = the collection name as-is; type & mutation names are PascalCase.)

### 5.2 Querying — pick your fields

```graphql
query {
  blog_posts(limit: 10, page: 1, search: "launch") {
    total
    page
    limit
    data {
      id
      title
      status
      created_at
    }
  }
}
```

`search` is a **keyword search** across the collection. Result shape is always
`{ data, total, page, limit }`.

Single item:

```graphql
query {
  blog_postsByID(id: "8f3c…") {
    id
    title
    body
  }
}
```

Send it with curl:

```bash
curl -s -H "Authorization: Bearer $TOKEN" -X POST https://cms.imaginadevs.com/api/graphql \
  -H 'Content-Type: application/json' \
  -d '{"query":"{ blog_posts(limit:5,search:\"launch\"){ total data{ id title } } }"}'
```

### 5.3 Mutations

```graphql
mutation {
  createBlogPosts(data: { title: "Hello", status: "draft" }) {
    id
    title
  }
}

mutation {
  updateBlogPosts(id: "8f3c…", data: { status: "published" }) {
    id
    status
  }
}

mutation {
  deleteBlogPosts(id: "8f3c…")   # returns true on success
}
```

Required fields are non-null in the `Input` type — omitting one is a validation
error.

### 5.4 Variables (recommended for real clients)

```json
{
  "query": "query($q:String,$n:Int){ products(search:$q, limit:$n){ data{ id title } total } }",
  "variables": { "q": "shoe", "n": 20 }
}
```

### 5.5 GraphQL type mapping

| Field type | GraphQL type |
|---|---|
| `integer`, `timestamp` | `Int` |
| `float`, `decimal` | `Float` |
| `boolean` | `Boolean` |
| `uuid`, `relation` | `ID` |
| everything else | `String` |

> **Errors:** GraphQL always responds `200`; check the `errors` array in the body.

---

## 6. REST vs GraphQL — capability matrix

| Capability | REST | GraphQL |
|---|---|---|
| Choose exact fields returned | ✗ (returns all) | ✓ |
| Field equality filter (`filter[x]=y`) | ✓ | ✗ |
| Keyword search | ✗ | ✓ (`search`) |
| Sort (`sort=-created_at`) | ✓ | ✗ |
| Pagination (`page`, `limit`) | ✓ | ✓ |
| Expand relations (`deep`) | ✓ | ✗ |
| Language (`lang`) | ✓ | ✗ |
| Nested create (write a graph) | ✓ | ✗ |
| Create / Update / Delete | ✓ | ✓ |

**Rule of thumb:** use **REST** when you need precise filtering, sorting,
relations or language; use **GraphQL** when you want to fetch a specific shape of
fields or do keyword search.

---

## 7. Status codes & errors

| Code | Meaning |
|---|---|
| `200` / `201` | Success |
| `204` | Success, no body (delete) |
| `400` | Bad request (invalid body, filter, or sort field) |
| `401` | Not authenticated (log in / cookie missing/expired) |
| `403` | Authenticated but not permitted for this collection/action |
| `404` | Tenant, collection, or item not found |

REST errors are JSON: `{ "error": "message" }`. GraphQL errors are in the
`errors` array of a `200` response.

---

## 8. Quick start checklist

1. `POST /api/auth/login` → keep the `access_token`; send it as `Authorization: Bearer`.
2. `GET /api/collections` → find your collection names.
3. `GET /api/collections/{name}` → learn its field names/types.
4. Read with **REST** (`filter[]`, `sort`, `deep`, `lang`) or **GraphQL**
   (`search`, choose fields).
5. Write with `POST` / `PATCH` / `DELETE` (REST) or `create*/update*/delete*`
   (GraphQL).
