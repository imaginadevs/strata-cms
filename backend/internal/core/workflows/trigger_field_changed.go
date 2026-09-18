package workflows

import (
	"fmt"
	"reflect"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/events"
)

type fieldChangedTrigger struct{}

func NewFieldChangedTrigger() TriggerMatcher { return &fieldChangedTrigger{} }

func (t *fieldChangedTrigger) Type() wfdomain.TriggerType { return wfdomain.TriggerFieldChanged }

func (t *fieldChangedTrigger) ValidateConfig(cfg map[string]any) error {
	if mapString(cfg, "collection") == "" {
		return errf("field_changed: collection is required")
	}
	if mapString(cfg, "field") == "" {
		return errf("field_changed: field is required")
	}
	if _, ok := cfg["to"]; !ok {
		return errf("field_changed: to is required")
	}
	return nil
}

func (t *fieldChangedTrigger) Match(cfg map[string]any, ev events.Event) (bool, error) {
	coll := mapString(cfg, "collection")
	if coll == "" || ev.Collection != coll {
		return false, nil
	}
	field := mapString(cfg, "field")
	if field == "" {
		return false, nil
	}

	changed, _ := ev.Meta["changed_fields"].([]string)
	if changed == nil {
		// JSON round-trip may produce []any
		if raw, ok := ev.Meta["changed_fields"].([]any); ok {
			for _, x := range raw {
				changed = append(changed, fmt.Sprint(x))
			}
		}
	}
	found := false
	for _, c := range changed {
		if c == field {
			found = true
			break
		}
	}
	if !found {
		// Also accept if values differ even without meta (defensive).
		if !reflect.DeepEqual(ev.Old[field], ev.Payload[field]) {
			found = true
		}
	}
	if !found {
		return false, nil
	}

	newVal := ev.Payload[field]
	to := cfg["to"]
	if !valuesEqual(newVal, to) {
		return false, nil
	}
	oldVal := ev.Old[field]
	if valuesEqual(oldVal, to) {
		return false, nil // already equal — not an edge
	}
	if from, ok := cfg["from"]; ok && from != nil {
		if !valuesEqual(oldVal, from) {
			return false, nil
		}
	}
	return true, nil
}

func valuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return reflect.DeepEqual(fmt.Sprint(a), fmt.Sprint(b)) || reflect.DeepEqual(a, b)
}
