# Collection lifecycle hooks

Every write to `/api/items/{collection}` publishes lifecycle events on the
core event bus (`internal/core/events`). Subscribe to run custom logic before
or after a create/update/delete — validation, side effects, cache
invalidation, webhooks, notifications, workflow triggers, etc.

This is different from [HTTP hooks](http-hooks.md), which register a brand
new endpoint outside the items CRUD surface.

## Event types

```go
events.EventBeforeCreate // items.before.create
events.EventAfterCreate  // items.after.create
events.EventBeforeUpdate // items.before.update
events.EventAfterUpdate  // items.after.update
events.EventBeforeDelete // items.before.delete
events.EventAfterDelete  // items.after.delete
```

Each `events.Event` carries `TenantID`, `Collection`, `UserID` (nil for
unauthenticated/Public requests), `ItemID`, `Old` (previous row, on
update/delete), and `Payload` (the request body being written, or the
row just written on `after.*`).

## Before vs. after: sync vs. async

- **`before.*`** is published with `bus.Publish`, **synchronously**, in the
  request's own goroutine, *before* the row is written. A handler that
  returns an error **aborts the write**: the HTTP handler maps the error to
  `400 Bad Request`, or `403 Forbidden` if the error wraps
  `events.ErrForbidden` (`fmt.Errorf("not allowed: %w", events.ErrForbidden)`).
  Use this for validation and authorization that can't be expressed as a
  static permission filter.

- **`after.*`** is published with `bus.PublishAfter`, which runs every
  subscriber **in its own goroutine** (bounded by an internal semaphore of
  32) and returns immediately — it never adds latency to the HTTP response,
  and a handler error is only logged, never surfaced to the client. Use this
  for anything that shouldn't block the response: outbound webhooks,
  notifications, search indexing, workflow triggers.

  The context passed to an `after.*` handler is detached from the request's
  cancellation (`context.WithoutCancel`) so it keeps running after the
  response has been written, but still carries the same request-scoped
  values.

## Subscribing

```go
bus.Subscribe(events.EventBeforeCreate, func(ctx context.Context, event events.Event) error {
    if event.Collection != "cmms_tasks" {
        return nil // only care about one collection
    }
    if event.Payload["title"] == "" {
        return fmt.Errorf("title is required")
    }
    return nil
})

bus.Subscribe(events.EventAfterCreate, func(ctx context.Context, event events.Event) error {
    if event.Collection != "ecommerce_orders" {
        return nil
    }
    return notifyWarehouse(ctx, event.Payload) // runs in its own goroutine, doesn't block the response
})
```

Subscribe during server wiring (see how `workflows.Engine.Start(bus)` and
`internal/core/notifications` do it) so the subscription exists before the
first request arrives.

## Field hashing and redaction

Fields declared with `type: "hash"` (e.g. `password_hash` on any `auth`-type
collection) are bcrypt-hashed automatically on create/update when present in
the request body, and stripped from every item response (list/get/create/
update) — hook payloads on `before.*` still see the plaintext value (useful
for e.g. password-strength validation), but by the time `after.*` fires the
stored value is already hashed and the emitted `Payload` is the persisted
(hashed, then redacted) row.

## Row-level permission filters

A role's `Permission.Filter` (set via `PUT /roles/{id}/permissions`) can
restrict which rows a request may see or write, e.g.:

```json
{ "customer_id": { "_eq": "$CURRENT_USER" } }
```

`$CURRENT_USER` resolves to the caller's id (a `cms_users` row, or an "auth"
principal row id from a custom auth collection like `ecommerce_customers`);
`$CURRENT_COLLECTION` resolves to the current collection name. The resolved
filter is:

- merged into `list`/`get` queries (server-enforced values win over
  client-supplied `filter[...]` query params for the same field),
- used to re-check ownership before `update`/`delete` (a non-owner gets
  `404 Not Found`, not `403`, to avoid leaking existence), and
- force-applied onto the payload on `create` (so a customer can't set
  `customer_id` to someone else's id).

Only the equality form (`{"field": "literal"}` or
`{"field": {"_eq": "literal-or-token"}}`) is supported — the underlying item
query engine only expresses equality filters.
