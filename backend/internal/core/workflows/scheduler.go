package workflows

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// Scheduler ticks active schedule workflows for all tenants.
type Scheduler struct {
	engine *Engine
	parser cron.Parser
}

func NewScheduler(engine *Engine) *Scheduler {
	return &Scheduler{
		engine: engine,
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor),
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) tick(ctx context.Context) {
	active := true
	tenants, _, err := s.engine.tenants.List(ctx, outbound.TenantFilter{Active: &active, Page: 1, Limit: 1000})
	if err != nil {
		s.engine.log.Errorw("workflow scheduler list tenants failed", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, t := range tenants {
		s.tickTenant(ctx, t.ID, now)
	}
}

func (s *Scheduler) tickTenant(ctx context.Context, tenantID uuid.UUID, now time.Time) {
	list, err := s.engine.repo.ListActiveByTrigger(ctx, tenantID, wfdomain.TriggerSchedule)
	if err != nil {
		s.engine.log.Errorw("workflow scheduler list failed", "err", err, "tenant_id", tenantID)
		return
	}
	for _, wf := range list {
		if !s.isDue(wf, now) {
			continue
		}
		busy, err := s.engine.repo.HasActiveRun(ctx, tenantID, wf.ID)
		if err != nil {
			s.engine.log.Warnw("workflow has-active check failed", "err", err)
			continue
		}
		if busy {
			continue
		}

		coll := mapString(wf.Trigger.Config, "collection")
		filter := map[string]any{}
		if m := nestedMap(wf.Trigger.Config, "filter"); m != nil {
			filter = m
		}
		limit := 50
		if v, ok := wf.Trigger.Config["limit"].(float64); ok && int(v) > 0 {
			limit = int(v)
		} else if v, ok := wf.Trigger.Config["limit"].(int); ok && v > 0 {
			limit = v
		}
		if limit > 200 {
			limit = 200
		}

		_ = s.engine.repo.SetLastFiredAt(ctx, tenantID, wf.ID, now)

		items, _, err := s.engine.items.List(ctx, tenantID, coll, outbound.QueryFilter{
			Filter: filter,
			Page:   1,
			Limit:  limit,
		})
		if err != nil {
			s.engine.log.Errorw("workflow schedule list items failed", "err", err, "workflow_id", wf.ID)
			continue
		}
		if len(items) == 0 {
			continue
		}
		for _, item := range items {
			s.engine.EnqueueScheduleItem(ctx, tenantID, wf, item)
		}
	}
}

func (s *Scheduler) isDue(wf *wfdomain.Workflow, now time.Time) bool {
	expr := mapString(wf.Trigger.Config, "cron")
	if expr == "" {
		return false
	}
	sched, err := s.parser.Parse(expr)
	if err != nil {
		s.engine.log.Warnw("invalid cron expression", "workflow_id", wf.ID, "cron", expr, "err", err)
		return false
	}

	tzName := mapString(wf.Trigger.Config, "timezone")
	loc := time.UTC
	if tzName != "" {
		if l, err := time.LoadLocation(tzName); err == nil {
			loc = l
		}
	}
	nowLocal := now.In(loc)

	from := nowLocal.Add(-time.Minute)
	if wf.LastFiredAt != nil {
		lf := wf.LastFiredAt.In(loc)
		if lf.After(from) {
			from = lf
		}
	}
	next := sched.Next(from)
	return !next.After(nowLocal)
}
