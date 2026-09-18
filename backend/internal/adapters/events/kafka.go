package events

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-kafka/v3/pkg/kafka"
	"github.com/ThreeDotsLabs/watermill/message"

	coreevents "github.com/imaginacolombia/cms/internal/core/events"
)

// KafkaBus implements core/events.Bus using Watermill + Kafka.
// Swap InMemoryBus for KafkaBus in main.go to enable distributed events.
//
// Any service (e.g. a webhook service, a search indexer, an AI pipeline)
// can subscribe to the same Kafka topics independently.
type KafkaBus struct {
	publisher  *kafka.Publisher
	subscriber *kafka.Subscriber
	topicPrefix string
	handlers    map[coreevents.EventType][]coreevents.Handler
	logger      watermill.LoggerAdapter
}

// NewKafkaBus creates a Watermill Kafka publisher+subscriber pair.
// brokers: comma-separated Kafka bootstrap servers.
// topicPrefix: e.g. "cms" → topics like "cms.items.after.create".
func NewKafkaBus(brokers []string, topicPrefix string) (*KafkaBus, error) {
	logger := watermill.NewStdLogger(false, false)

	pub, err := kafka.NewPublisher(
		kafka.PublisherConfig{
			Brokers:   brokers,
			Marshaler: kafka.DefaultMarshaler{},
		},
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("kafka publisher: %w", err)
	}

	sub, err := kafka.NewSubscriber(
		kafka.SubscriberConfig{
			Brokers:       brokers,
			Unmarshaler:   kafka.DefaultMarshaler{},
			ConsumerGroup: topicPrefix + ".cms",
		},
		logger,
	)
	if err != nil {
		pub.Close() //nolint:errcheck
		return nil, fmt.Errorf("kafka subscriber: %w", err)
	}

	return &KafkaBus{
		publisher:   pub,
		subscriber:  sub,
		topicPrefix: topicPrefix,
		handlers:    make(map[coreevents.EventType][]coreevents.Handler),
		logger:      logger,
	}, nil
}

func (b *KafkaBus) Publish(ctx context.Context, event coreevents.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	topic := b.topic(event.Type)
	msg := message.NewMessage(watermill.NewUUID(), payload)

	if err := b.publisher.Publish(topic, msg); err != nil {
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}

	// Also dispatch to in-process handlers (same as InMemoryBus)
	for _, h := range b.handlers[event.Type] {
		if err := h(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// PublishAfter publishes to Kafka (fire-and-forget from the caller's point of
// view — the broker write still happens synchronously) then runs in-process
// handlers in detached goroutines, mirroring InMemoryBus.PublishAfter.
func (b *KafkaBus) PublishAfter(ctx context.Context, event coreevents.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	topic := b.topic(event.Type)
	msg := message.NewMessage(watermill.NewUUID(), payload)
	if err := b.publisher.Publish(topic, msg); err != nil {
		return fmt.Errorf("kafka publish to %s: %w", topic, err)
	}

	handlers := b.handlers[event.Type]
	if len(handlers) == 0 {
		return nil
	}
	detached := context.WithoutCancel(ctx)
	for _, h := range handlers {
		h := h
		go func() {
			defer func() {
				if r := recover(); r != nil {
					b.logger.Error("panic in after-handler", fmt.Errorf("%v", r), nil)
				}
			}()
			if err := h(detached, event); err != nil {
				b.logger.Error("after-handler failed", err, nil)
			}
		}()
	}
	return nil
}

func (b *KafkaBus) Subscribe(eventType coreevents.EventType, handler coreevents.Handler) coreevents.Subscription {
	// In-process subscription (for synchronous before-hooks).
	// For async consumers use StartConsumer() with a dedicated goroutine.
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return coreevents.Subscription{}
}

func (b *KafkaBus) Unsubscribe(_ coreevents.Subscription) {
	// In-process handlers are long-lived; Kafka consumers are managed separately.
}

// StartConsumer subscribes to a Kafka topic and calls handler for each message.
// Run this in a goroutine. Blocks until ctx is cancelled.
func (b *KafkaBus) StartConsumer(ctx context.Context, eventType coreevents.EventType, handler coreevents.Handler) error {
	msgs, err := b.subscriber.Subscribe(ctx, b.topic(eventType))
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	for msg := range msgs {
		var event coreevents.Event
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			msg.Nack()
			continue
		}
		if err := handler(ctx, event); err != nil {
			msg.Nack()
		} else {
			msg.Ack()
		}
	}
	return nil
}

func (b *KafkaBus) Close() error {
	_ = b.subscriber.Close()
	return b.publisher.Close()
}

func (b *KafkaBus) topic(eventType coreevents.EventType) string {
	return b.topicPrefix + "." + string(eventType)
}
