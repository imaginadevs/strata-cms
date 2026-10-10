package features

import (
	"context"

	"github.com/google/uuid"

	wfdomain "github.com/imaginacolombia/cms/internal/core/domain/workflow"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// WorkflowSpec is a default workflow a plugin installs for a tenant on enable.
type WorkflowSpec struct {
	Name        string
	TriggerType wfdomain.TriggerType
	TriggerCfg  map[string]any
	Actions     []wfdomain.Action
}

// WorkflowsForFeature returns the default workflows a plugin provisions.
func WorkflowsForFeature(f FeatureID) []WorkflowSpec {
	switch f {
	case FeatureBlog:
		return []WorkflowSpec{
			{
				Name:        "Notify admins when a blog post is created",
				TriggerType: wfdomain.TriggerEntityCreated,
				TriggerCfg:  map[string]any{"collection": "blog_posts"},
				Actions: []wfdomain.Action{
					{
						Type:   wfdomain.ActionSendTemplateEmail,
						Config: map[string]any{"template_key": EmailKeyBlogEntryCreated, "to": "admins"},
					},
				},
			},
		}
	default:
		return nil
	}
}

// EnsureFeatureWorkflows installs a plugin's default workflows for a tenant,
// idempotently: if any workflow already carries this plugin's origin tag, it
// assumes they're installed and does nothing. Tag = the feature id.
func EnsureFeatureWorkflows(
	ctx context.Context,
	tenantID uuid.UUID,
	feature FeatureID,
	repo outbound.WorkflowRepository,
) error {
	specs := WorkflowsForFeature(feature)
	if len(specs) == 0 {
		return nil
	}
	existing, err := repo.CountByOrigin(ctx, tenantID, string(feature))
	if err != nil {
		return err
	}
	if existing > 0 {
		return nil // already provisioned
	}
	origin := string(feature)
	for _, sp := range specs {
		wf := wfdomain.New(sp.Name, wfdomain.Trigger{Type: sp.TriggerType, Config: sp.TriggerCfg}, sp.Actions)
		wf.OriginPlugin = &origin
		if err := repo.Create(ctx, tenantID, wf); err != nil {
			return err
		}
	}
	return nil
}
