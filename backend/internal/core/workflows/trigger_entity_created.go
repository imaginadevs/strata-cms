package workflows

import (
	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/events"
)

// entityCreatedTrigger fires when an item is created in the configured collection.
// Config: { "collection": "<name>" } — required.
type entityCreatedTrigger struct{}

func NewEntityCreatedTrigger() TriggerMatcher { return &entityCreatedTrigger{} }

func (t *entityCreatedTrigger) Type() wfdomain.TriggerType { return wfdomain.TriggerEntityCreated }

func (t *entityCreatedTrigger) ValidateConfig(cfg map[string]any) error {
	if mapString(cfg, "collection") == "" {
		return errf("entity_created: collection is required")
	}
	return nil
}

func (t *entityCreatedTrigger) Match(cfg map[string]any, ev events.Event) (bool, error) {
	coll := mapString(cfg, "collection")
	if coll == "" || ev.Collection != coll {
		return false, nil
	}
	return true, nil
}
