package rest

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/domain/ids"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/core/workflows"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountWorkflows(r chi.Router, repos *postgresrepo.Repos, engine *workflows.Engine, log *logger.Logger) {
	h := &workflowHandler{repos: repos, engine: engine, log: log}
	p := repos.Permission
	r.Route("/workflows", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionRead)).Get("/catalog", h.catalog)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionRead)).Get("/runs/{runId}", h.getRun)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionRead)).Get("/{id}", h.getByID)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionDelete)).Delete("/{id}", h.delete)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualWorkflows, userdomain.ActionRead)).Get("/{id}/runs", h.listRuns)
	})
}

type workflowHandler struct {
	repos  *postgresrepo.Repos
	engine *workflows.Engine
	log    *logger.Logger
}

type workflowActionBody struct {
	ID     *uuid.UUID     `json:"id"`
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

type workflowBody struct {
	Name    string `json:"name"`
	Active  *bool  `json:"active"`
	Trigger struct {
		Type   string         `json:"type"`
		Config map[string]any `json:"config"`
	} `json:"trigger"`
	Actions []workflowActionBody `json:"actions"`
}


func (h *workflowHandler) catalog(w http.ResponseWriter, _ *http.Request) {
	triggers, actions := h.engine.Registry().Catalog()
	writeJSON(w, http.StatusOK, map[string]any{
		"triggers": triggers,
		"actions":  actions,
	})
}

func (h *workflowHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	page, limit := parsePage(r)
	filter := outbound.WorkflowFilter{Page: page, Limit: limit}
	if a := r.URL.Query().Get("active"); a == "true" || a == "1" {
		v := true
		filter.Active = &v
	} else if a == "false" || a == "0" {
		v := false
		filter.Active = &v
	}
	if tt := r.URL.Query().Get("trigger_type"); tt != "" {
		t := wfdomain.TriggerType(tt)
		filter.TriggerType = &t
	}
	list, total, err := h.repos.Workflow.List(r.Context(), tenantID, filter)
	if err != nil {
		h.log.Errorw("workflow list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, list, total, page, limit)
}

func (h *workflowHandler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	wf, err := h.repos.Workflow.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if wf == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, wf)
}

func (h *workflowHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	var body workflowBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	trigger, actions, err := h.parseDefinition(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.engine.Registry().ValidateWorkflow(trigger, actions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	wf := wfdomain.New(body.Name, trigger, actions)
	if body.Active != nil {
		wf.Active = *body.Active
	}
	if err := h.repos.Workflow.Create(r.Context(), tenantID, wf); err != nil {
		h.log.Errorw("workflow create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, wf)
}

func (h *workflowHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	existing, err := h.repos.Workflow.GetByID(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if existing == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	var body workflowBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Active != nil {
		existing.Active = *body.Active
	}
	if body.Trigger.Type != "" || body.Actions != nil {
		trigger, actions, err := h.parseDefinition(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Trigger.Type != "" {
			existing.Trigger = trigger
		}
		if body.Actions != nil {
			existing.Actions = actions
		}
	}
	if err := h.engine.Registry().ValidateWorkflow(existing.Trigger, existing.Actions); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.repos.Workflow.Update(r.Context(), tenantID, existing); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Errorw("workflow update failed", "err", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *workflowHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.repos.Workflow.Delete(r.Context(), tenantID, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *workflowHandler) listRuns(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	page, limit := parsePage(r)
	filter := outbound.WorkflowRunFilter{WorkflowID: &id, Page: page, Limit: limit}
	if st := r.URL.Query().Get("status"); st != "" {
		s := wfdomain.RunStatus(st)
		filter.Status = &s
	}
	list, total, err := h.repos.Workflow.ListRuns(r.Context(), tenantID, filter)
	if err != nil {
		h.log.Errorw("workflow runs list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, list, total, page, limit)
}

func (h *workflowHandler) getRun(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	runID, err := uuid.Parse(chi.URLParam(r, "runId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid run id")
		return
	}
	run, err := h.repos.Workflow.GetRunByID(r.Context(), tenantID, runID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if run == nil {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (h *workflowHandler) parseDefinition(body workflowBody) (wfdomain.Trigger, []wfdomain.Action, error) {
	trigger := wfdomain.Trigger{
		Type:   wfdomain.TriggerType(body.Trigger.Type),
		Config: body.Trigger.Config,
	}
	if trigger.Config == nil {
		trigger.Config = map[string]any{}
	}
	actions := make([]wfdomain.Action, 0, len(body.Actions))
	for i, a := range body.Actions {
		id := ids.New()
		if a.ID != nil {
			id = *a.ID
		}
		cfg := a.Config
		if cfg == nil {
			cfg = map[string]any{}
		}
		actions = append(actions, wfdomain.Action{
			ID:     id,
			Type:   wfdomain.ActionType(a.Type),
			Sort:   i,
			Config: cfg,
		})
	}
	return trigger, actions, nil
}
