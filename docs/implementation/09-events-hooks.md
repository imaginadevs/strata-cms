# 09 — Events & Hooks

## Overview

The event bus decouples item lifecycle operations from secondary effects (notifications, search indexing, cache invalidation). Handlers that subscribe before an operation (guard hooks) can abort it by returning an error. Handlers that run after (side-effect hooks) perform async work. The default `InMemoryBus` is swapped for `KafkaBus` by changing two lines in `main.go`. This guide covers the full hook pattern, event types, Kafka setup, and consumer implementation.

## Prerequisites

- Guide 08 completed (REST items endpoint publishing events).
- For Kafka: Docker Compose with Kafka running, `ENABLE_KAFKA=true` in `.env`.

## Step-by-Step

### 1. Event bus interface recap

```go
// From backend/internal/core/events/bus.go
type Bus interface {
    Publish(ctx context.Context, event Event) error
    Subscribe(eventType EventType, handler Handler) Subscription
    Unsubscribe(sub Subscription)
}

// Handler returning non-nil error aborts the operation (guard hook).
type Handler func(ctx context.Context, event Event) error
```

Event types relevant to hooks:

| Event | When fired | Guard capable |
|-------|-----------|---------------|
| `items.before.create` | Before DB insert | Yes |
| `items.after.create`  | After DB insert  | No (side-effect only) |
| `items.before.update` | Before DB update | Yes |
| `items.after.update`  | After DB update  | No |
| `items.before.delete` | Before DB delete | Yes |
| `items.after.delete`  | After DB delete  | No |
| `schema.collection.created` | After DDL | No |
| `auth.login` | After successful login | No |

### 2. Guard hook — reject invalid data before create

Guard hooks run synchronously in the `Publish` call. If the handler returns an error, `Publish` returns that error and the handler aborts the operation:

```go
// backend/internal/adapters/hooks/validation_hooks.go
package hooks

import (
    "context"
    "fmt"
    "regexp"

    "github.com/imaginacolombia/cms/internal/core/events"
)

var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// RegisterValidationHooks wires guard hooks for common validation rules.
// Call this once at startup before the server starts serving requests.
func RegisterValidationHooks(bus events.Bus) {
    // Guard: validate "posts" slug format before create
    bus.Subscribe(events.EventBeforeCreate, func(ctx context.Context, e events.Event) error {
        if e.Collection != "posts" {
            return nil // only applies to posts
        }
        slug, _ := e.Payload["slug"].(string)
        if slug == "" {
            return fmt.Errorf("posts.slug is required")
        }
        if !slugRegex.MatchString(slug) {
            return fmt.Errorf("posts.slug must be lowercase alphanumeric with hyphens only, got %q", slug)
        }
        return nil
    })

    // Guard: prevent update of published posts by non-admins
    bus.Subscribe(events.EventBeforeUpdate, func(ctx context.Context, e events.Event) error {
        if e.Collection != "posts" {
            return nil
        }
        if old, ok := e.Old["status"].(string); ok && old == "published" {
            // Check if user has admin role (simplified — check JWT roles in ctx)
            roles, _ := ctx.Value("user_roles").([]string)
            for _, r := range roles {
                if r == "Administrator" || r == "super_admin" {
                    return nil
                }
            }
            return fmt.Errorf("only Administrators can update published posts")
        }
        return nil
    })

    // Guard: prevent deletion of system collections
    bus.Subscribe(events.EventBeforeDelete, func(ctx context.Context, e events.Event) error {
        if e.Meta != nil {
            if isSystem, _ := e.Meta["is_system"].(bool); isSystem {
                return fmt.Errorf("cannot delete system collection %q", e.Collection)
            }
        }
        return nil
    })
}
```

Register at startup in `main.go`:

```go
bus := events.NewInMemoryBus()
hooks.RegisterValidationHooks(bus)

srv := server.New(cfg, log, db, bus)
```

### 3. Side-effect hook — notifications after create

Side-effect hooks run after the database operation. They should not affect the response and should handle their own errors gracefully:

```go
// backend/internal/adapters/hooks/notification_hooks.go
package hooks

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "strings"
    "time"

    "go.uber.org/zap"

    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// RegisterNotificationHooks wires side-effect hooks for async notifications.
func RegisterNotificationHooks(bus events.Bus, log *logger.Logger, webhookURL string) {
    bus.Subscribe(events.EventAfterCreate, func(ctx context.Context, e events.Event) error {
        // Run asynchronously to avoid blocking the HTTP response
        go func() {
            if err := sendWebhook(webhookURL, e, log); err != nil {
                log.Errorw("webhook delivery failed",
                    zap.String("collection", e.Collection),
                    zap.Error(err),
                )
            }
        }()
        return nil // never return error from a side-effect hook
    })

    bus.Subscribe(events.EventAfterCreate, func(ctx context.Context, e events.Event) error {
        // Search index update example
        if e.Collection == "posts" {
            go indexDocumentForSearch(e)
        }
        return nil
    })
}

func sendWebhook(url string, e events.Event, log *logger.Logger) error {
    if url == "" {
        return nil
    }

    payload, err := json.Marshal(e)
    if err != nil {
        return err
    }

    client := &http.Client{Timeout: 10 * time.Second}
    req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(payload)))
    req.Header.Set("Content-Type", "application/json")

    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode >= 400 {
        return fmt.Errorf("webhook returned %d", resp.StatusCode)
    }
    return nil
}

func indexDocumentForSearch(e events.Event) {
    // Placeholder — implement with Elasticsearch, Meilisearch, or pgvector
    // Example: POST /<index>/posts/<id> with e.Payload
}
```

### 4. Audit log hook — record every mutation

```go
// backend/internal/adapters/hooks/audit_hooks.go
package hooks

import (
    "context"

    "github.com/google/uuid"

    "github.com/imaginacolombia/cms/internal/core/domain/audit"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

func RegisterAuditHooks(bus events.Bus, auditRepo outbound.AuditRepository) {
    writeAudit := func(op audit.Operation) events.Handler {
        return func(ctx context.Context, e events.Event) error {
            var itemID *string
            if e.ItemID != nil {
                itemID = e.ItemID
            }

            log := &audit.Log{
                ID:         uuid.New(),
                TenantID:   e.TenantID,
                UserID:     e.UserID,
                UserEmail:  e.UserEmail,
                Collection: e.Collection,
                ItemID:     itemID,
                Operation:  op,
                OldData:    e.Old,
                NewData:    e.Payload,
                CreatedAt:  time.Now().UTC(),
            }

            // Extract IP from meta
            if e.Meta != nil {
                if ip, ok := e.Meta["ip"].(string); ok {
                    log.IPAddress = &ip
                }
            }

            // Write async — audit log failure should not affect main operation
            go func() {
                _ = auditRepo.Create(context.Background(), log)
            }()

            return nil
        }
    }

    bus.Subscribe(events.EventAfterCreate, writeAudit(audit.OperationCreate))
    bus.Subscribe(events.EventAfterUpdate, writeAudit(audit.OperationUpdate))
    bus.Subscribe(events.EventAfterDelete, writeAudit(audit.OperationDelete))
    bus.Subscribe(events.EventUserLogin, writeAudit(audit.OperationLogin))
    bus.Subscribe(events.EventUserLogout, writeAudit(audit.OperationLogout))
}
```

### 5. Hook registration pattern — central registry

Create a single registration function that wires all hooks in order:

```go
// backend/internal/adapters/hooks/registry.go
package hooks

import (
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
    "github.com/imaginacolombia/cms/internal/infrastructure/config"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// Register wires all hooks into the event bus.
// Guard hooks (before.*) must be registered before side-effect hooks (after.*).
func Register(
    bus events.Bus,
    cfg *config.Config,
    auditRepo outbound.AuditRepository,
    log *logger.Logger,
) {
    // 1. Guard hooks — can abort operations
    RegisterValidationHooks(bus)

    // 2. Audit hook — always runs
    RegisterAuditHooks(bus, auditRepo)

    // 3. Notification hooks — best-effort
    webhookURL := cfg.App.WebhookURL // add to config if needed
    RegisterNotificationHooks(bus, log, webhookURL)
}
```

In `main.go`:

```go
auditRepo := postgresAdapter.NewAuditRepository(db)
hooks.Register(bus, cfg, auditRepo, log)
```

### 6. Switching from InMemoryBus to KafkaBus

The `KafkaBus` in `backend/internal/adapters/events/kafka.go` implements the same `Bus` interface. Switching is a two-line change in `main.go`:

```go
// main.go (before — in-memory)
bus := events.NewInMemoryBus()

// main.go (after — Kafka)
var bus events.Bus
if cfg.Features.Kafka {
    kafkaBus, err := kafkaAdapter.NewKafkaBus(cfg.Kafka.Brokers, cfg.Kafka.TopicPrefix)
    if err != nil {
        log.Fatal(fmt.Sprintf("kafka bus init failed: %v", err))
    }
    defer kafkaBus.Close()
    bus = kafkaBus
} else {
    bus = events.NewInMemoryBus()
}
```

Enable in `.env`:

```dotenv
ENABLE_KAFKA=true
KAFKA_BROKERS=localhost:9092
KAFKA_TOPIC_PREFIX=cms
```

Topics created automatically by the publisher follow the pattern:

```
cms.items.before.create
cms.items.after.create
cms.items.after.update
cms.items.after.delete
cms.auth.login
cms.tenant.created
```

### 7. Writing a Kafka consumer with StartConsumer()

The `KafkaBus.StartConsumer()` method blocks and processes messages from a Kafka topic. Run it in a goroutine:

```go
// backend/cmd/consumer/main.go
package main

import (
    "context"
    "fmt"
    "os"
    "os/signal"
    "syscall"

    kafkaAdapter "github.com/imaginacolombia/cms/internal/adapters/events"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/imaginacolombia/cms/internal/infrastructure/config"
    "github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer cancel()

    cfg := config.Load()
    log := logger.New(cfg.App.Environment)

    bus, err := kafkaAdapter.NewKafkaBus(cfg.Kafka.Brokers, cfg.Kafka.TopicPrefix)
    if err != nil {
        log.Fatal(fmt.Sprintf("kafka: %v", err))
    }
    defer bus.Close()

    // Consumer 1: search indexer for posts
    go func() {
        err := bus.StartConsumer(ctx, events.EventAfterCreate, func(ctx context.Context, e events.Event) error {
            if e.Collection != "posts" {
                return nil
            }
            log.Infow("indexing post", "id", e.ItemID)
            // TODO: send to Meilisearch / Elasticsearch
            return nil
        })
        if err != nil {
            log.Errorw("search consumer error", "err", err)
        }
    }()

    // Consumer 2: cache invalidation
    go func() {
        err := bus.StartConsumer(ctx, events.EventAfterUpdate, func(ctx context.Context, e events.Event) error {
            log.Infow("cache invalidate", "collection", e.Collection, "id", e.ItemID)
            // TODO: delete from Redis / CDN edge cache
            return nil
        })
        if err != nil {
            log.Errorw("cache consumer error", "err", err)
        }
    }()

    log.Info("consumers running — waiting for events")
    <-ctx.Done()
    log.Info("consumers shutting down")
}
```

Run separately from the main server:

```bash
cd backend && go run ./cmd/consumer/main.go
```

Or add to `docker-compose.yml` as a separate service using the same binary with a different command.

### 8. InMemoryBus vs KafkaBus behavior

| Behavior | InMemoryBus | KafkaBus |
|----------|------------|---------|
| Before hooks (guard) | Synchronous, blocks response | Synchronous (in-process handlers only) |
| After hooks (side-effect) | Synchronous (same goroutine) | Kafka publish is async; in-process handlers run sync |
| Multi-service fanout | No | Yes — any service can subscribe to the topic |
| Replay | No | Yes — consumers can reset offset |
| Ordering guarantee | FIFO | Per-partition order |
| Failure mode | Bus error stops handler chain | Kafka unavailable → publish fails → 500 response |

### 9. Testing hooks in isolation

```go
// backend/internal/adapters/hooks/validation_hooks_test.go
package hooks_test

import (
    "context"
    "testing"

    "github.com/imaginacolombia/cms/internal/adapters/hooks"
    "github.com/imaginacolombia/cms/internal/core/events"
    "github.com/google/uuid"
)

func TestSlugValidationHook(t *testing.T) {
    bus := events.NewInMemoryBus()
    hooks.RegisterValidationHooks(bus)

    tenantID := uuid.New()

    // Valid slug — should pass
    err := bus.Publish(context.Background(), events.Event{
        Type:       events.EventBeforeCreate,
        Collection: "posts",
        TenantID:   tenantID,
        Payload:    map[string]any{"slug": "hello-world", "title": "Hello"},
    })
    if err != nil {
        t.Fatalf("expected no error for valid slug, got: %v", err)
    }

    // Invalid slug — should be rejected
    err = bus.Publish(context.Background(), events.Event{
        Type:       events.EventBeforeCreate,
        Collection: "posts",
        TenantID:   tenantID,
        Payload:    map[string]any{"slug": "Hello World!", "title": "Hello"},
    })
    if err == nil {
        t.Fatal("expected error for invalid slug, got nil")
    }
}
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/core/events/bus.go` — Bus interface + InMemoryBus
- `/home/jd/dev/full/cms/backend/internal/core/events/events.go` — EventType constants + Event struct
- `/home/jd/dev/full/cms/backend/internal/adapters/events/kafka.go` — KafkaBus (Watermill + Kafka)
- `/home/jd/dev/full/cms/backend/cmd/server/main.go` — bus selection logic

## Next Steps

Proceed to [10 — i18n](./10-i18n.md) to implement translatable fields, the shadow table pattern, language fallback, and the `?lang=` query parameter.
