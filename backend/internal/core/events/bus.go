package events

import (
	"context"
	"fmt"
	"os"
	"sync"
)

// afterConcurrency bounds how many PublishAfter handler goroutines may run at
// once, protecting downstream resources (DB pool, outbound HTTP) from an
// unbounded fan-out when a single event has many subscribers.
const afterConcurrency = 32

// Bus is the central event dispatcher. Adapters and services publish events;
// hooks registered by collections (or external integrations) subscribe to them.
type Bus interface {
	// Publish runs handlers synchronously, in the request's goroutine. Used for
	// "before.*" events, where a handler error must abort the operation (guard).
	Publish(ctx context.Context, event Event) error
	// PublishAfter runs each handler in its own goroutine (bounded by an internal
	// semaphore) and returns immediately without waiting for them. Used for
	// "after.*" events, where side effects (webhooks, notifications, workflow
	// triggers) must never add latency to — or fail — the HTTP response.
	// Handler errors are logged by the bus, never surfaced to the caller.
	PublishAfter(ctx context.Context, event Event) error
	Subscribe(eventType EventType, handler Handler) Subscription
	Unsubscribe(sub Subscription)
}

// Handler is a function invoked synchronously before the response is sent.
// Returning an error aborts the operation (acts as a guard hook).
type Handler func(ctx context.Context, event Event) error

// Subscription is a handle returned by Subscribe; pass it to Unsubscribe to stop.
type Subscription struct {
	id        uint64
	eventType EventType
}

// InMemoryBus is the default synchronous, in-process implementation.
// Replace with a distributed bus (e.g. NATS, Watermill+Kafka) without
// changing any caller code — just swap the Bus implementation wired in main.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[EventType][]subscriber
	nextID   uint64
	sem      chan struct{}
	onError  func(event Event, err error)
}

type subscriber struct {
	id      uint64
	handler Handler
}

func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[EventType][]subscriber),
		sem:      make(chan struct{}, afterConcurrency),
		onError: func(event Event, err error) {
			fmt.Fprintf(os.Stderr, "events: after-handler error type=%s collection=%s: %v\n", event.Type, event.Collection, err)
		},
	}
}

// SetErrorHandler overrides how PublishAfter reports handler errors (default:
// stderr). Lets infrastructure wiring (main.go) route these into the app's
// structured logger without core/events importing it.
func (b *InMemoryBus) SetErrorHandler(fn func(event Event, err error)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if fn != nil {
		b.onError = fn
	}
}

func (b *InMemoryBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	subs := make([]subscriber, len(b.handlers[event.Type]))
	copy(subs, b.handlers[event.Type])
	b.mu.RUnlock()

	for _, s := range subs {
		if err := s.handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// PublishAfter dispatches each handler in its own goroutine, bounded by a
// semaphore, and returns immediately. The handler context is detached from
// ctx's cancellation (via context.WithoutCancel) so handlers keep running
// after the HTTP response has been written, but still carry request-scoped
// values (tenant, actor, etc.) for handlers that need them.
func (b *InMemoryBus) PublishAfter(ctx context.Context, event Event) error {
	b.mu.RLock()
	subs := make([]subscriber, len(b.handlers[event.Type]))
	copy(subs, b.handlers[event.Type])
	onError := b.onError
	b.mu.RUnlock()

	if len(subs) == 0 {
		return nil
	}

	detached := context.WithoutCancel(ctx)
	for _, s := range subs {
		s := s
		b.sem <- struct{}{}
		go func() {
			defer func() { <-b.sem }()
			defer func() {
				if r := recover(); r != nil {
					onError(event, fmt.Errorf("panic: %v", r))
				}
			}()
			if err := s.handler(detached, event); err != nil {
				onError(event, err)
			}
		}()
	}
	return nil
}

func (b *InMemoryBus) Subscribe(eventType EventType, handler Handler) Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := b.nextID
	b.handlers[eventType] = append(b.handlers[eventType], subscriber{id: id, handler: handler})

	return Subscription{id: id, eventType: eventType}
}

func (b *InMemoryBus) Unsubscribe(sub Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.handlers[sub.eventType]
	for i, s := range subs {
		if s.id == sub.id {
			b.handlers[sub.eventType] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}
