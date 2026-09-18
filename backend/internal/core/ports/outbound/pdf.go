package outbound

import (
	"context"

	"github.com/google/uuid"
)

type PDFRenderRequest struct {
	TenantID   uuid.UUID
	Collection string
	Template   string
	Filename   string
	ItemID     string
	Item       map[string]any
}

type PDFRenderer interface {
	Render(ctx context.Context, req PDFRenderRequest) (filename string, pdf []byte, err error)
}
