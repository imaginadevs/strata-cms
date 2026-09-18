// Package notifications is the app-wide entry point for creating inbox items.
//
// Wire once (typically near router construction):
//
//	svc := notifications.NewService(repos.Notification, bus)
//
// Then from any use-case / handler:
//
//	_, err := svc.ToUser(ctx, tenantID, recipientID, notifications.Input{
//	    Title: "Role updated",
//	    Body:  "Your role was changed to Editor",
//	    Type:  notification.TypeInfo,
//	    Link:  ptr("/settings/profile"),
//	    ActorID: &actorID,
//	})
//
// ToUser persists the row and publishes EventNotificationCreated so the
// recipient's SSE stream (/api/events) receives it immediately.
package notifications

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	notifdomain "github.com/imaginacolombia/cms/internal/core/domain/notification"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// Input is the payload used when creating a notification from anywhere in the app.
type Input struct {
	Title   string
	Body    string
	Type    notifdomain.Type
	Link    *string
	Meta    map[string]any
	ActorID *uuid.UUID
}

// Service persists notifications and publishes EventNotificationCreated for SSE.
type Service struct {
	repo outbound.NotificationRepository
	bus  events.Bus
}

func NewService(repo outbound.NotificationRepository, bus events.Bus) *Service {
	return &Service{repo: repo, bus: bus}
}

// ToUser creates one notification for a single recipient and pushes via the event bus.
func (s *Service) ToUser(ctx context.Context, tenantID, recipientID uuid.UUID, in Input) (*notifdomain.Notification, error) {
	if in.Title == "" {
		return nil, fmt.Errorf("notification title is required")
	}
	n := notifdomain.New(recipientID, in.Title, in.Type)
	n.Body = in.Body
	n.Link = in.Link
	n.ActorID = in.ActorID
	if in.Meta != nil {
		n.Meta = in.Meta
	}
	if err := s.repo.Create(ctx, tenantID, n); err != nil {
		return nil, err
	}
	_ = s.bus.Publish(ctx, events.Event{
		Type:     events.EventNotificationCreated,
		TenantID: tenantID,
		UserID:   &recipientID,
		Payload: map[string]any{
			"id":           n.ID.String(),
			"recipient_id": n.RecipientID.String(),
			"type":         string(n.Type),
			"title":        n.Title,
			"body":         n.Body,
			"link":         n.Link,
			"meta":         n.Meta,
			"created_at":   n.CreatedAt,
		},
	})
	return n, nil
}

// ToUsers notifies multiple recipients (best-effort; returns the first error).
func (s *Service) ToUsers(ctx context.Context, tenantID uuid.UUID, recipientIDs []uuid.UUID, in Input) error {
	var first error
	for _, id := range recipientIDs {
		if _, err := s.ToUser(ctx, tenantID, id, in); err != nil && first == nil {
			first = err
		}
	}
	return first
}
