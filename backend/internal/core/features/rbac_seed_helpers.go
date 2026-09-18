package features

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	domain "github.com/imaginacolombia/cms/internal/core/domain"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// ensureRole returns the role with the given name, creating it (as a system
// role) if missing. Idempotent.
func ensureRole(ctx context.Context, tenantID uuid.UUID, name, description string, roleRepo outbound.RoleRepository) (*userdomain.Role, error) {
	existing, err := roleRepo.GetByName(ctx, tenantID, name)
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", name, err)
	}
	if existing != nil {
		return existing, nil
	}
	role := &userdomain.Role{
		Base:        domain.NewBase(tenantID, nil),
		Name:        name,
		Description: description,
		IsSystem:    true,
	}
	if err := roleRepo.Create(ctx, tenantID, role); err != nil {
		return nil, fmt.Errorf("create role %q: %w", name, err)
	}
	return role, nil
}

// upsertPermission grants roleID the given action on collection, with
// optional field restriction and row-level filter. Idempotent (Upsert is
// keyed by role_id+collection+action).
func upsertPermission(
	ctx context.Context,
	tenantID, roleID uuid.UUID,
	collection string,
	action userdomain.Action,
	fields []string,
	filter any,
	permRepo outbound.PermissionRepository,
) error {
	p := &userdomain.Permission{
		Base:       domain.NewBase(tenantID, nil),
		RoleID:     roleID,
		Collection: collection,
		Action:     action,
		Fields:     fields,
		Filter:     filter,
	}
	if err := permRepo.Upsert(ctx, tenantID, p); err != nil {
		return fmt.Errorf("grant %s.%s to role %s: %w", collection, action, roleID, err)
	}
	return nil
}

// setAuthDefaultRole stamps collections.meta.auth_default_role on an
// auth-type collection, so /auth/{collection}/login knows which role name to
// put on issued tokens. No-op if the collection is missing (plugin not
// installed yet) or already set to the desired value.
func setAuthDefaultRole(ctx context.Context, tenantID uuid.UUID, collectionName, roleName string, collRepo outbound.CollectionRepository) error {
	c, err := collRepo.GetByName(ctx, tenantID, collectionName)
	if err != nil {
		return fmt.Errorf("get collection %q: %w", collectionName, err)
	}
	if c == nil {
		return nil
	}
	if existing, _ := c.Meta["auth_default_role"].(string); existing == roleName {
		return nil
	}
	if c.Meta == nil {
		c.Meta = map[string]any{}
	}
	c.Meta["auth_default_role"] = roleName
	if err := collRepo.Update(ctx, tenantID, c); err != nil {
		return fmt.Errorf("set auth_default_role on %q: %w", collectionName, err)
	}
	return nil
}
