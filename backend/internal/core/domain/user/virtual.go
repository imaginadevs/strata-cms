package user

// Virtual system collections used in permissions.collection.
// They are not real CMS tables and must never be creatable via POST /collections.
const (
	VirtualUsers    = "cms_users"    // Users admin
	VirtualRoles    = "cms_roles"    // Roles & permissions
	VirtualSettings = "cms_settings" // Tenant configuration
	VirtualSchema   = "cms_schema"   // Collections / fields / sections / relations / indexes
	VirtualAudit     = "cms_audit"     // Audit logs
	VirtualWorkflows = "cms_workflows" // Workflow definitions & runs
)

// ActionAll grants every action on a collection when stored as permissions.action.
const ActionAll Action = "all"

// VirtualCollection describes a system permission target for the frontend catalog.
type VirtualCollection struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Actions     []Action `json:"actions"`
}

// AllActions is the full set the permission editor should offer (including "all").
func AllActions() []Action {
	return []Action{ActionCreate, ActionRead, ActionUpdate, ActionDelete, ActionAll}
}

// VirtualCatalog is the fixed list of non-content permission targets.
func VirtualCatalog() []VirtualCollection {
	actions := AllActions()
	return []VirtualCollection{
		{Name: VirtualUsers, DisplayName: "Users", Description: "Manage tenant users", Actions: actions},
		{Name: VirtualRoles, DisplayName: "Roles", Description: "Manage roles and permissions", Actions: actions},
		{Name: VirtualSettings, DisplayName: "Tenant Configuration", Description: "Tenant settings and feature flags", Actions: actions},
		{Name: VirtualSchema, DisplayName: "Collection Configuration", Description: "Collections, fields, sections, relations, indexes", Actions: actions},
		{Name: VirtualAudit, DisplayName: "Audit Logs", Description: "View audit history", Actions: actions},
		{Name: VirtualWorkflows, DisplayName: "Workflows", Description: "Automations, triggers, and run history", Actions: actions},
	}
}

// IsVirtualCollection reports whether name is a reserved virtual permission key.
func IsVirtualCollection(name string) bool {
	switch name {
	case VirtualUsers, VirtualRoles, VirtualSettings, VirtualSchema, VirtualAudit, VirtualWorkflows:
		return true
	default:
		return false
	}
}