package features

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// SeedCMMSRBAC provisions the "CMMS Operator" role for the cmms_operators
// auth collection: operators can read their assigned tasks/assets/forms and
// create submissions, but only for tasks assigned to them. Also ensures the
// "Public" role exists (defence in depth — the tenant migration already
// seeds it, but plugin install may run standalone, e.g. in tests).
//
// Safe to call repeatedly (idempotent).
func SeedCMMSRBAC(
	ctx context.Context,
	tenantID uuid.UUID,
	roleRepo outbound.RoleRepository,
	permRepo outbound.PermissionRepository,
	collRepo outbound.CollectionRepository,
) error {
	if roleRepo == nil || permRepo == nil {
		return nil
	}

	if _, err := ensureRole(ctx, tenantID, "Public", "Unauthenticated access", roleRepo); err != nil {
		return fmt.Errorf("cmms rbac: %w", err)
	}

	operatorRole, err := ensureRole(ctx, tenantID, "CMMS Operator", "Field technician", roleRepo)
	if err != nil {
		return fmt.Errorf("cmms rbac: %w", err)
	}

	// Read-only reference data needed to work a task.
	for _, coll := range []string{"cmms_assets", "cmms_forms", "cmms_form_versions"} {
		if err := upsertPermission(ctx, tenantID, operatorRole.ID, coll, userdomain.ActionRead, nil, nil, permRepo); err != nil {
			return fmt.Errorf("cmms rbac: operator read %s: %w", coll, err)
		}
	}

	// Only tasks assigned to this operator.
	ownTasks := map[string]any{"assigned_to": map[string]any{"_eq": "$CURRENT_USER"}}
	for _, action := range []userdomain.Action{userdomain.ActionRead, userdomain.ActionUpdate} {
		if err := upsertPermission(ctx, tenantID, operatorRole.ID, "cmms_tasks", action, nil, ownTasks, permRepo); err != nil {
			return fmt.Errorf("cmms rbac: operator tasks %s: %w", action, err)
		}
	}

	// Only their own submissions.
	ownSubmissions := map[string]any{"operator_id": map[string]any{"_eq": "$CURRENT_USER"}}
	for _, action := range []userdomain.Action{userdomain.ActionRead, userdomain.ActionCreate} {
		if err := upsertPermission(ctx, tenantID, operatorRole.ID, "cmms_submissions", action, nil, ownSubmissions, permRepo); err != nil {
			return fmt.Errorf("cmms rbac: operator submissions %s: %w", action, err)
		}
	}

	if collRepo != nil {
		if err := setAuthDefaultRole(ctx, tenantID, "cmms_operators", operatorRole.Name, collRepo); err != nil {
			return fmt.Errorf("cmms rbac: %w", err)
		}
	}
	return nil
}
