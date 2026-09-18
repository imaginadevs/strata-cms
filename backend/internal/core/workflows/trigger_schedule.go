package workflows

import (
	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/events"
)

type scheduleTrigger struct{}

func NewScheduleTrigger() TriggerMatcher { return &scheduleTrigger{} }

func (t *scheduleTrigger) Type() wfdomain.TriggerType { return wfdomain.TriggerSchedule }

func (t *scheduleTrigger) ValidateConfig(cfg map[string]any) error {
	if mapString(cfg, "cron") == "" {
		return errf("schedule: cron is required")
	}
	if mapString(cfg, "collection") == "" {
		return errf("schedule: collection is required")
	}
	return nil
}

func (t *scheduleTrigger) Match(_ map[string]any, _ events.Event) (bool, error) {
	return false, nil // driven by scheduler, not the event bus
}
