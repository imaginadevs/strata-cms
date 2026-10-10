package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/email"
)

// emailTemplateRepository stores per-tenant email template overrides and
// implements email.TemplateStore (and therefore email.OverrideStore).
type emailTemplateRepository struct{ db *pgxpool.Pool }

// Get returns the override for a key, or nil when none exists or it's disabled
// (so the built-in default is used).
func (r *emailTemplateRepository) Get(ctx context.Context, tenantID uuid.UUID, key string) (*email.Template, error) {
	s := schemaOf(tenantID)
	var t email.Template
	var enabled bool
	err := r.db.QueryRow(ctx, fmt.Sprintf(
		`SELECT subject, html, text, enabled FROM %s.email_templates WHERE template_key=$1`, s), key).
		Scan(&t.Subject, &t.HTML, &t.Text, &enabled)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	return &t, nil
}

func (r *emailTemplateRepository) Upsert(ctx context.Context, tenantID uuid.UUID, key string, t email.Template, enabled bool, actor *uuid.UUID) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`
		INSERT INTO %s.email_templates (template_key, subject, html, text, enabled, updated_by, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW())
		ON CONFLICT (template_key) DO UPDATE
		SET subject=EXCLUDED.subject, html=EXCLUDED.html, text=EXCLUDED.text,
		    enabled=EXCLUDED.enabled, updated_by=EXCLUDED.updated_by, updated_at=NOW()`, s),
		key, t.Subject, t.HTML, t.Text, enabled, actor)
	return err
}

func (r *emailTemplateRepository) List(ctx context.Context, tenantID uuid.UUID) ([]email.StoredTemplate, error) {
	s := schemaOf(tenantID)
	rows, err := r.db.Query(ctx, fmt.Sprintf(
		`SELECT template_key, subject, html, text, enabled FROM %s.email_templates ORDER BY template_key`, s))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []email.StoredTemplate
	for rows.Next() {
		var st email.StoredTemplate
		if err := rows.Scan(&st.Key, &st.Subject, &st.HTML, &st.Text, &st.Enabled); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

func (r *emailTemplateRepository) Delete(ctx context.Context, tenantID uuid.UUID, key string) error {
	s := schemaOf(tenantID)
	_, err := r.db.Exec(ctx, fmt.Sprintf(`DELETE FROM %s.email_templates WHERE template_key=$1`, s), key)
	return err
}
