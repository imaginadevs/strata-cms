package workflow

import (
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

type TriggerType string

const (
	TriggerFieldChanged TriggerType = "field_changed"
	TriggerSchedule     TriggerType = "schedule"
)

type ActionType string

const (
	ActionEmailWithPDF ActionType = "email_with_pdf"
)

type RunStatus string

const (
	RunPending RunStatus = "pending"
	RunRunning RunStatus = "running"
	RunSuccess RunStatus = "success"
	RunFailed  RunStatus = "failed"
)

type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepRunning StepStatus = "running"
	StepSuccess StepStatus = "success"
	StepFailed  StepStatus = "failed"
	StepSkipped StepStatus = "skipped"
)

type Trigger struct {
	Type   TriggerType     `json:"type"`
	Config map[string]any  `json:"config"`
}

type Action struct {
	ID     uuid.UUID      `json:"id"`
	Type   ActionType     `json:"type"`
	Sort   int            `json:"sort"`
	Config map[string]any `json:"config"`
}

type Workflow struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	Active      bool       `json:"active"`
	Trigger     Trigger    `json:"trigger"`
	Actions     []Action   `json:"actions"`
	LastFiredAt *time.Time `json:"last_fired_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func New(name string, trigger Trigger, actions []Action) *Workflow {
	now := time.Now().UTC()
	for i := range actions {
		if actions[i].ID == uuid.Nil {
			actions[i].ID = ids.New()
		}
		if actions[i].Config == nil {
			actions[i].Config = map[string]any{}
		}
		actions[i].Sort = i
	}
	return &Workflow{
		ID:        ids.New(),
		Name:      name,
		Active:    true,
		Trigger:   trigger,
		Actions:   actions,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

type Run struct {
	ID          uuid.UUID      `json:"id"`
	WorkflowID  uuid.UUID      `json:"workflow_id"`
	Status      RunStatus      `json:"status"`
	Collection  string         `json:"collection"`
	ItemID      string         `json:"item_id"`
	TriggerType TriggerType    `json:"trigger_type"`
	Error       string         `json:"error,omitempty"`
	Context     map[string]any `json:"context,omitempty"`
	Steps       []RunStep      `json:"steps,omitempty"`
	StartedAt   time.Time      `json:"started_at"`
	FinishedAt  *time.Time     `json:"finished_at,omitempty"`
}

type RunStep struct {
	ID         uuid.UUID      `json:"id"`
	RunID      uuid.UUID      `json:"run_id"`
	ActionID   uuid.UUID      `json:"action_id"`
	ActionType ActionType     `json:"action_type"`
	Sort       int            `json:"sort"`
	Status     StepStatus     `json:"status"`
	Error      string         `json:"error,omitempty"`
	Output     map[string]any `json:"output,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
}

func NewRun(workflowID uuid.UUID, triggerType TriggerType, collection, itemID string, ctx map[string]any, actions []Action) *Run {
	now := time.Now().UTC()
	run := &Run{
		ID:          ids.New(),
		WorkflowID:  workflowID,
		Status:      RunPending,
		Collection:  collection,
		ItemID:      itemID,
		TriggerType: triggerType,
		Context:     ctx,
		StartedAt:   now,
	}
	for _, a := range actions {
		run.Steps = append(run.Steps, RunStep{
			ID:         ids.New(),
			RunID:      run.ID,
			ActionID:   a.ID,
			ActionType: a.Type,
			Sort:       a.Sort,
			Status:     StepPending,
			Output:     map[string]any{},
			StartedAt:  now,
		})
	}
	return run
}
