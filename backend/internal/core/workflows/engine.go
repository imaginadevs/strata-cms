package workflows

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

type Engine struct {
	repo     outbound.WorkflowRepository
	items    outbound.ItemRepository
	tenants  outbound.TenantRepository
	registry *Registry
	log      *logger.Logger
	workers  chan struct{}
	mu       sync.Mutex
	subs     []events.Subscription
}

func NewEngine(
	repo outbound.WorkflowRepository,
	items outbound.ItemRepository,
	tenants outbound.TenantRepository,
	registry *Registry,
	log *logger.Logger,
	concurrency int,
) *Engine {
	if concurrency < 1 {
		concurrency = 4
	}
	return &Engine{
		repo:     repo,
		items:    items,
		tenants:  tenants,
		registry: registry,
		log:      log,
		workers:  make(chan struct{}, concurrency),
	}
}

func (e *Engine) Registry() *Registry { return e.registry }

func (e *Engine) Start(bus events.Bus) {
	sub := bus.Subscribe(events.EventAfterUpdate, func(ctx context.Context, ev events.Event) error {
		e.onItemUpdate(ctx, ev)
		return nil
	})
	subCreate := bus.Subscribe(events.EventAfterCreate, func(ctx context.Context, ev events.Event) error {
		e.onItemCreate(ctx, ev)
		return nil
	})
	e.mu.Lock()
	e.subs = append(e.subs, sub, subCreate)
	e.mu.Unlock()
}

// onItemCreate dispatches entity_created workflows for a newly created item.
func (e *Engine) onItemCreate(ctx context.Context, ev events.Event) {
	list, err := e.repo.ListActiveByTrigger(ctx, ev.TenantID, wfdomain.TriggerEntityCreated)
	if err != nil {
		e.log.Errorw("workflow list (entity_created) failed", "err", err)
		return
	}
	matcher, ok := e.registry.Trigger(wfdomain.TriggerEntityCreated)
	if !ok {
		return
	}
	itemID := ""
	if ev.ItemID != nil {
		itemID = *ev.ItemID
	}
	for _, wf := range list {
		match, err := matcher.Match(wf.Trigger.Config, ev)
		if err != nil {
			e.log.Warnw("workflow match error", "workflow_id", wf.ID, "err", err)
			continue
		}
		if !match {
			continue
		}
		runCtx := map[string]any{"item": ev.Payload}
		e.enqueueRun(ctx, ev.TenantID, wf, itemID, ev.Collection, ev.Payload, nil, nil, ev.UserID, runCtx)
	}
}

func (e *Engine) Stop(bus events.Bus) {
	e.mu.Lock()
	subs := append([]events.Subscription{}, e.subs...)
	e.subs = nil
	e.mu.Unlock()
	for _, s := range subs {
		bus.Unsubscribe(s)
	}
}

func (e *Engine) onItemUpdate(ctx context.Context, ev events.Event) {
	list, err := e.repo.ListActiveByTrigger(ctx, ev.TenantID, wfdomain.TriggerFieldChanged)
	if err != nil {
		e.log.Errorw("workflow list failed", "err", err)
		return
	}
	matcher, ok := e.registry.Trigger(wfdomain.TriggerFieldChanged)
	if !ok {
		return
	}
	itemID := ""
	if ev.ItemID != nil {
		itemID = *ev.ItemID
	}
	changed, _ := ev.Meta["changed_fields"].([]string)

	for _, wf := range list {
		match, err := matcher.Match(wf.Trigger.Config, ev)
		if err != nil {
			e.log.Warnw("workflow match error", "workflow_id", wf.ID, "err", err)
			continue
		}
		if !match {
			continue
		}
		runCtx := map[string]any{
			"item":           ev.Payload,
			"old":            ev.Old,
			"changed_fields": changed,
		}
		e.enqueueRun(ctx, ev.TenantID, wf, itemID, ev.Collection, ev.Payload, ev.Old, changed, ev.UserID, runCtx)
	}
}

func (e *Engine) enqueueRun(
	ctx context.Context,
	tenantID uuid.UUID,
	wf *wfdomain.Workflow,
	itemID, collection string,
	item, old map[string]any,
	changed []string,
	actorID *uuid.UUID,
	runCtx map[string]any,
) {
	run := wfdomain.NewRun(wf.ID, wf.Trigger.Type, collection, itemID, runCtx, wf.Actions)
	if err := e.repo.CreateRun(ctx, tenantID, run); err != nil {
		e.log.Errorw("workflow create run failed", "err", err, "workflow_id", wf.ID)
		return
	}

	// Capture for async execution.
	tenantID, wfID, runID := tenantID, wf.ID, run.ID
	actions := append([]wfdomain.Action{}, wf.Actions...)
	steps := append([]wfdomain.RunStep{}, run.Steps...)

	go func() {
		e.workers <- struct{}{}
		defer func() { <-e.workers }()

		bg := context.Background()
		e.execute(bg, tenantID, wfID, runID, collection, itemID, item, old, changed, actorID, actions, steps)
	}()
}

func (e *Engine) execute(
	ctx context.Context,
	tenantID, wfID, runID uuid.UUID,
	collection, itemID string,
	item, old map[string]any,
	changed []string,
	actorID *uuid.UUID,
	actions []wfdomain.Action,
	steps []wfdomain.RunStep,
) {
	now := time.Now().UTC()
	run := &wfdomain.Run{
		ID:         runID,
		WorkflowID: wfID,
		Status:     wfdomain.RunRunning,
		StartedAt:  now,
	}
	_ = e.repo.UpdateRun(ctx, tenantID, run)

	prior := map[string]any{}
	failed := false
	var failErr string

	for i := range steps {
		step := &steps[i]
		if failed {
			fin := time.Now().UTC()
			step.Status = wfdomain.StepSkipped
			step.FinishedAt = &fin
			_ = e.repo.UpdateRunStep(ctx, tenantID, step)
			continue
		}

		var action *wfdomain.Action
		for j := range actions {
			if actions[j].ID == step.ActionID {
				action = &actions[j]
				break
			}
		}
		if action == nil {
			failed = true
			failErr = fmt.Sprintf("action %s not found on workflow", step.ActionID)
			fin := time.Now().UTC()
			step.Status = wfdomain.StepFailed
			step.Error = failErr
			step.FinishedAt = &fin
			_ = e.repo.UpdateRunStep(ctx, tenantID, step)
			continue
		}

		ex, ok := e.registry.Action(action.Type)
		if !ok {
			failed = true
			failErr = fmt.Sprintf("unknown action type %q", action.Type)
			fin := time.Now().UTC()
			step.Status = wfdomain.StepFailed
			step.Error = failErr
			step.FinishedAt = &fin
			_ = e.repo.UpdateRunStep(ctx, tenantID, step)
			continue
		}

		start := time.Now().UTC()
		step.Status = wfdomain.StepRunning
		step.StartedAt = start
		_ = e.repo.UpdateRunStep(ctx, tenantID, step)

		out, err := ex.Execute(ctx, ActionInput{
			TenantID:   tenantID,
			WorkflowID: wfID,
			RunID:      runID,
			Collection: collection,
			ItemID:     itemID,
			Item:       item,
			Old:        old,
			Changed:    changed,
			ActorID:    actorID,
			Config:     action.Config,
			PriorOut:   prior,
		})
		fin := time.Now().UTC()
		step.FinishedAt = &fin
		if err != nil {
			failed = true
			failErr = err.Error()
			step.Status = wfdomain.StepFailed
			step.Error = failErr
			_ = e.repo.UpdateRunStep(ctx, tenantID, step)
			continue
		}
		step.Status = wfdomain.StepSuccess
		step.Output = out.Data
		if step.Output == nil {
			step.Output = map[string]any{}
		}
		for k, v := range step.Output {
			prior[k] = v
		}
		_ = e.repo.UpdateRunStep(ctx, tenantID, step)
	}

	fin := time.Now().UTC()
	run.FinishedAt = &fin
	if failed {
		run.Status = wfdomain.RunFailed
		run.Error = failErr
	} else {
		run.Status = wfdomain.RunSuccess
	}
	_ = e.repo.UpdateRun(ctx, tenantID, run)
}

// EnqueueScheduleItem is used by the cron scheduler to start a run for one item.
func (e *Engine) EnqueueScheduleItem(ctx context.Context, tenantID uuid.UUID, wf *wfdomain.Workflow, item map[string]any) {
	itemID := ""
	if raw, ok := item["id"]; ok && raw != nil {
		itemID = fmt.Sprint(raw)
	}
	coll := mapString(wf.Trigger.Config, "collection")
	runCtx := map[string]any{"item": item, "source": "schedule"}
	e.enqueueRun(ctx, tenantID, wf, itemID, coll, item, nil, nil, nil, runCtx)
}
