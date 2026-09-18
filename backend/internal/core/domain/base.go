package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

// Base is embedded in every domain entity to provide standard audit fields.
// Tenancy is enforced by the per-tenant Postgres schema, so entities carry no
// tenant_id column — the schema itself is the tenant boundary.
type Base struct {
	ID        uuid.UUID  `json:"id"         db:"id"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt time.Time  `json:"updated_at" db:"updated_at"`
	CreatedBy *uuid.UUID `json:"created_by" db:"created_by"`
	UpdatedBy *uuid.UUID `json:"updated_by" db:"updated_by"`
}

// NewBase initialises a new Base with a fresh UUID and current timestamp.
// tenantID is retained for call-site compatibility (it selects the schema at the
// repository layer) but is no longer stored on the entity.
func NewBase(tenantID uuid.UUID, actorID *uuid.UUID) Base {
	_ = tenantID
	now := time.Now().UTC()
	return Base{
		ID:        ids.New(),
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: actorID,
		UpdatedBy: actorID,
	}
}

// Touch updates UpdatedAt and UpdatedBy.
func (b *Base) Touch(actorID *uuid.UUID) {
	b.UpdatedAt = time.Now().UTC()
	b.UpdatedBy = actorID
}
