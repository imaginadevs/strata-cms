package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type workflowRepository struct{ db *pgxpool.Pool }

func (r *workflowRepository) Create(ctx context.Context, tenantID uuid.UUID, wf *wfdomain.Workflow) error {
	s := schemaOf(tenantID)
	trigJSON, _ := json.Marshal(wf.Trigger.Config)
	if wf.Trigger.Config == nil {
		trigJSON = []byte("{}")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.workflows (id, name, active, trigger_type, trigger_config, origin_plugin, last_fired_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, s),
		wf.ID, wf.Name, wf.Active, string(wf.Trigger.Type), trigJSON, wf.OriginPlugin, wf.LastFiredAt, wf.CreatedAt, wf.UpdatedAt,
	)
	if err != nil {
		return err
	}
	for _, a := range wf.Actions {
		cfgJSON, _ := json.Marshal(a.Config)
		if a.Config == nil {
			cfgJSON = []byte("{}")
		}
		_, err = tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.workflow_actions (id, workflow_id, sort, action_type, action_config)
			VALUES ($1,$2,$3,$4,$5)`, s),
			a.ID, wf.ID, a.Sort, string(a.Type), cfgJSON,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *workflowRepository) CountByOrigin(ctx context.Context, tenantID uuid.UUID, originPlugin string) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s.workflows WHERE origin_plugin=$1`, schemaOf(tenantID)),
		originPlugin).Scan(&n)
	return n, err
}

func (r *workflowRepository) Update(ctx context.Context, tenantID uuid.UUID, wf *wfdomain.Workflow) error {
	s := schemaOf(tenantID)
	trigJSON, _ := json.Marshal(wf.Trigger.Config)
	if wf.Trigger.Config == nil {
		trigJSON = []byte("{}")
	}
	wf.UpdatedAt = time.Now().UTC()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	tag, err := tx.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.workflows
		SET name=$2, active=$3, trigger_type=$4, trigger_config=$5, updated_at=$6
		WHERE id=$1`, s),
		wf.ID, wf.Name, wf.Active, string(wf.Trigger.Type), trigJSON, wf.UpdatedAt,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	_, err = tx.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.workflow_actions WHERE workflow_id=$1`, s), wf.ID)
	if err != nil {
		return err
	}
	for i := range wf.Actions {
		a := &wf.Actions[i]
		a.Sort = i
		cfgJSON, _ := json.Marshal(a.Config)
		if a.Config == nil {
			cfgJSON = []byte("{}")
		}
		_, err = tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.workflow_actions (id, workflow_id, sort, action_type, action_config)
			VALUES ($1,$2,$3,$4,$5)`, s),
			a.ID, wf.ID, a.Sort, string(a.Type), cfgJSON,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *workflowRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.workflows WHERE id=$1`, s), id)
	return err
}

func (r *workflowRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*wfdomain.Workflow, error) {
	s := schemaOf(tenantID)
	row := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, name, active, trigger_type, trigger_config, last_fired_at, created_at, updated_at
		FROM %s.workflows WHERE id=$1`, s), id)
	wf, err := scanWorkflow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	actions, err := r.loadActions(ctx, s, wf.ID)
	if err != nil {
		return nil, err
	}
	wf.Actions = actions
	return wf, nil
}

func (r *workflowRepository) List(ctx context.Context, tenantID uuid.UUID, filter outbound.WorkflowFilter) ([]*wfdomain.Workflow, int64, error) {
	s := schemaOf(tenantID)
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	where := "WHERE TRUE"
	args := []any{}
	if filter.Active != nil {
		args = append(args, *filter.Active)
		where += fmt.Sprintf(" AND active=$%d", len(args))
	}
	if filter.TriggerType != nil {
		args = append(args, string(*filter.TriggerType))
		where += fmt.Sprintf(" AND trigger_type=$%d", len(args))
	}

	var total int64
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s.workflows %s`, s, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, name, active, trigger_type, trigger_config, last_fired_at, created_at, updated_at
		FROM %s.workflows %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, s, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*wfdomain.Workflow
	for rows.Next() {
		wf, err := scanWorkflow(rows)
		if err != nil {
			return nil, 0, err
		}
		actions, err := r.loadActions(ctx, s, wf.ID)
		if err != nil {
			return nil, 0, err
		}
		wf.Actions = actions
		out = append(out, wf)
	}
	return out, total, rows.Err()
}

func (r *workflowRepository) ListActiveByTrigger(ctx context.Context, tenantID uuid.UUID, triggerType wfdomain.TriggerType) ([]*wfdomain.Workflow, error) {
	active := true
	list, _, err := r.List(ctx, tenantID, outbound.WorkflowFilter{
		Active:      &active,
		TriggerType: &triggerType,
		Page:        1,
		Limit:       500,
	})
	return list, err
}

func (r *workflowRepository) SetLastFiredAt(ctx context.Context, tenantID, id uuid.UUID, at time.Time) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`UPDATE %s.workflows SET last_fired_at=$2 WHERE id=$1`, s), id, at)
	return err
}

func (r *workflowRepository) CreateRun(ctx context.Context, tenantID uuid.UUID, run *wfdomain.Run) error {
	s := schemaOf(tenantID)
	ctxJSON, _ := json.Marshal(run.Context)
	if run.Context == nil {
		ctxJSON = []byte("{}")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	_, err = tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.workflow_runs
		    (id, workflow_id, status, collection, item_id, trigger_type, error, context, started_at, finished_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s),
		run.ID, run.WorkflowID, string(run.Status), run.Collection, run.ItemID,
		string(run.TriggerType), run.Error, ctxJSON, run.StartedAt, run.FinishedAt,
	)
	if err != nil {
		return err
	}
	for _, step := range run.Steps {
		outJSON, _ := json.Marshal(step.Output)
		if step.Output == nil {
			outJSON = []byte("{}")
		}
		_, err = tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO %s.workflow_run_steps
			    (id, run_id, action_id, action_type, sort, status, error, output, started_at, finished_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, s),
			step.ID, run.ID, step.ActionID, string(step.ActionType), step.Sort,
			string(step.Status), step.Error, outJSON, step.StartedAt, step.FinishedAt,
		)
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *workflowRepository) UpdateRun(ctx context.Context, tenantID uuid.UUID, run *wfdomain.Run) error {
	s := schemaOf(tenantID)
	ctxJSON, _ := json.Marshal(run.Context)
	if run.Context == nil {
		ctxJSON = []byte("{}")
	}
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.workflow_runs
		SET status=$2, error=$3, context=$4, finished_at=$5
		WHERE id=$1`, s),
		run.ID, string(run.Status), run.Error, ctxJSON, run.FinishedAt,
	)
	return err
}

func (r *workflowRepository) UpdateRunStep(ctx context.Context, tenantID uuid.UUID, step *wfdomain.RunStep) error {
	s := schemaOf(tenantID)
	outJSON, _ := json.Marshal(step.Output)
	if step.Output == nil {
		outJSON = []byte("{}")
	}
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		UPDATE %s.workflow_run_steps
		SET status=$2, error=$3, output=$4, started_at=$5, finished_at=$6
		WHERE id=$1`, s),
		step.ID, string(step.Status), step.Error, outJSON, step.StartedAt, step.FinishedAt,
	)
	return err
}

func (r *workflowRepository) GetRunByID(ctx context.Context, tenantID, id uuid.UUID) (*wfdomain.Run, error) {
	s := schemaOf(tenantID)
	row := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT id, workflow_id, status, collection, item_id, trigger_type, error, context, started_at, finished_at
		FROM %s.workflow_runs WHERE id=$1`, s), id)
	run, err := scanRun(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	steps, err := r.loadSteps(ctx, s, run.ID)
	if err != nil {
		return nil, err
	}
	run.Steps = steps
	return run, nil
}

func (r *workflowRepository) ListRuns(ctx context.Context, tenantID uuid.UUID, filter outbound.WorkflowRunFilter) ([]*wfdomain.Run, int64, error) {
	s := schemaOf(tenantID)
	page, limit := normalisePage(filter.Page, filter.Limit)
	offset := (page - 1) * limit

	where := "WHERE TRUE"
	args := []any{}
	if filter.WorkflowID != nil {
		args = append(args, *filter.WorkflowID)
		where += fmt.Sprintf(" AND workflow_id=$%d", len(args))
	}
	if filter.Status != nil {
		args = append(args, string(*filter.Status))
		where += fmt.Sprintf(" AND status=$%d", len(args))
	}

	var total int64
	if err := r.db.QueryRow(ctx, fmt.Sprintf(`SELECT COUNT(*) FROM %s.workflow_runs %s`, s, where), args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, workflow_id, status, collection, item_id, trigger_type, error, context, started_at, finished_at
		FROM %s.workflow_runs %s
		ORDER BY started_at DESC
		LIMIT $%d OFFSET $%d`, s, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []*wfdomain.Run
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, run)
	}
	return out, total, rows.Err()
}

func (r *workflowRepository) HasActiveRun(ctx context.Context, tenantID, workflowID uuid.UUID) (bool, error) {
	s := schemaOf(tenantID)
	var exists bool
	err := r.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT EXISTS(
			SELECT 1 FROM %s.workflow_runs
			WHERE workflow_id=$1 AND status IN ('pending','running')
		)`, s), workflowID).Scan(&exists)
	return exists, err
}

func (r *workflowRepository) loadActions(ctx context.Context, schema string, workflowID uuid.UUID) ([]wfdomain.Action, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, action_type, sort, action_config
		FROM %s.workflow_actions WHERE workflow_id=$1 ORDER BY sort`, schema), workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []wfdomain.Action
	for rows.Next() {
		var a wfdomain.Action
		var typ string
		var cfgRaw []byte
		if err := rows.Scan(&a.ID, &typ, &a.Sort, &cfgRaw); err != nil {
			return nil, err
		}
		a.Type = wfdomain.ActionType(typ)
		_ = json.Unmarshal(cfgRaw, &a.Config)
		if a.Config == nil {
			a.Config = map[string]any{}
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *workflowRepository) loadSteps(ctx context.Context, schema string, runID uuid.UUID) ([]wfdomain.RunStep, error) {
	rows, err := r.db.Query(ctx, fmt.Sprintf(`
		SELECT id, run_id, action_id, action_type, sort, status, error, output, started_at, finished_at
		FROM %s.workflow_run_steps WHERE run_id=$1 ORDER BY sort`, schema), runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []wfdomain.RunStep
	for rows.Next() {
		var st wfdomain.RunStep
		var typ, status string
		var outRaw []byte
		if err := rows.Scan(&st.ID, &st.RunID, &st.ActionID, &typ, &st.Sort, &status, &st.Error, &outRaw, &st.StartedAt, &st.FinishedAt); err != nil {
			return nil, err
		}
		st.ActionType = wfdomain.ActionType(typ)
		st.Status = wfdomain.StepStatus(status)
		_ = json.Unmarshal(outRaw, &st.Output)
		if st.Output == nil {
			st.Output = map[string]any{}
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func scanWorkflow(row rowScanner) (*wfdomain.Workflow, error) {
	var wf wfdomain.Workflow
	var trigType string
	var cfgRaw []byte
	err := row.Scan(&wf.ID, &wf.Name, &wf.Active, &trigType, &cfgRaw, &wf.LastFiredAt, &wf.CreatedAt, &wf.UpdatedAt)
	if err != nil {
		return nil, err
	}
	wf.Trigger.Type = wfdomain.TriggerType(trigType)
	_ = json.Unmarshal(cfgRaw, &wf.Trigger.Config)
	if wf.Trigger.Config == nil {
		wf.Trigger.Config = map[string]any{}
	}
	return &wf, nil
}

func scanRun(row rowScanner) (*wfdomain.Run, error) {
	var run wfdomain.Run
	var status, trigType string
	var ctxRaw []byte
	err := row.Scan(&run.ID, &run.WorkflowID, &status, &run.Collection, &run.ItemID, &trigType, &run.Error, &ctxRaw, &run.StartedAt, &run.FinishedAt)
	if err != nil {
		return nil, err
	}
	run.Status = wfdomain.RunStatus(status)
	run.TriggerType = wfdomain.TriggerType(trigType)
	_ = json.Unmarshal(ctxRaw, &run.Context)
	if run.Context == nil {
		run.Context = map[string]any{}
	}
	return &run, nil
}
