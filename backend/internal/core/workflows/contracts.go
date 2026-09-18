package workflows

import (
	"context"

	"github.com/google/uuid"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/events"
)

type ActionInput struct {
	TenantID   uuid.UUID
	WorkflowID uuid.UUID
	RunID      uuid.UUID
	Collection string
	ItemID     string
	Item       map[string]any
	Old        map[string]any
	Changed    []string
	ActorID    *uuid.UUID
	Config     map[string]any
	PriorOut   map[string]any
}

type ActionOutput struct {
	Data map[string]any
}

type ActionExecutor interface {
	Type() wfdomain.ActionType
	ValidateConfig(cfg map[string]any) error
	Execute(ctx context.Context, in ActionInput) (ActionOutput, error)
}

type TriggerMatcher interface {
	Type() wfdomain.TriggerType
	ValidateConfig(cfg map[string]any) error
	Match(cfg map[string]any, ev events.Event) (bool, error)
}

type Registry struct {
	triggers map[wfdomain.TriggerType]TriggerMatcher
	actions  map[wfdomain.ActionType]ActionExecutor
}

func NewRegistry() *Registry {
	return &Registry{
		triggers: map[wfdomain.TriggerType]TriggerMatcher{},
		actions:  map[wfdomain.ActionType]ActionExecutor{},
	}
}

func (r *Registry) RegisterTrigger(m TriggerMatcher) { r.triggers[m.Type()] = m }
func (r *Registry) RegisterAction(a ActionExecutor)   { r.actions[a.Type()] = a }

func (r *Registry) Trigger(t wfdomain.TriggerType) (TriggerMatcher, bool) {
	m, ok := r.triggers[t]
	return m, ok
}

func (r *Registry) Action(t wfdomain.ActionType) (ActionExecutor, bool) {
	a, ok := r.actions[t]
	return a, ok
}

func (r *Registry) ValidateWorkflow(trigger wfdomain.Trigger, actions []wfdomain.Action) error {
	tm, ok := r.Trigger(trigger.Type)
	if !ok {
		return errf("unknown trigger type %q", trigger.Type)
	}
	if err := tm.ValidateConfig(trigger.Config); err != nil {
		return err
	}
	if len(actions) == 0 {
		return errf("workflow requires at least one action")
	}
	for _, a := range actions {
		ex, ok := r.Action(a.Type)
		if !ok {
			return errf("unknown action type %q", a.Type)
		}
		if err := ex.ValidateConfig(a.Config); err != nil {
			return err
		}
	}
	return nil
}
