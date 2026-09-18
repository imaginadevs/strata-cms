package notification

import (
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

// Type is a coarse category for UI icons / styling.
type Type string

const (
	TypeInfo    Type = "info"
	TypeSuccess Type = "success"
	TypeWarning Type = "warning"
	TypeError   Type = "error"
)

// Notification is a per-user inbox item within a tenant schema.
type Notification struct {
	ID          uuid.UUID      `json:"id"`
	RecipientID uuid.UUID      `json:"recipient_id"`
	ActorID     *uuid.UUID     `json:"actor_id,omitempty"`
	Type        Type           `json:"type"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	Link        *string        `json:"link,omitempty"`
	Meta        map[string]any `json:"meta,omitempty"`
	ReadAt      *time.Time     `json:"read_at,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// New builds an unread notification ready to persist.
func New(recipientID uuid.UUID, title string, nType Type) *Notification {
	if nType == "" {
		nType = TypeInfo
	}
	return &Notification{
		ID:          ids.New(),
		RecipientID: recipientID,
		Type:        nType,
		Title:       title,
		Body:        "",
		Meta:        map[string]any{},
		CreatedAt:   time.Now().UTC(),
	}
}

func (n *Notification) IsRead() bool { return n.ReadAt != nil }