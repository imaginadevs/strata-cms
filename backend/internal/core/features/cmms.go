package features

import collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"

// cmmsCollections defines the CMMS (Computerized Maintenance Management
// System) plugin blueprint: assets, dynamic inspection forms (versioned),
// scheduled/ad-hoc tasks against those assets, field submissions, and a
// dedicated "operators" auth collection so field technicians authenticate
// separately from back-office cms_users.
func cmmsCollections() []Blueprint {
	return []Blueprint{
		{
			Name: "cmms_assets", DisplayName: "Assets", Icon: "wrench", Color: "#0ea5e9",
			Fields: []FieldBlueprint{
				{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "code", DisplayName: "Code", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				{Name: "location", DisplayName: "Location", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)"},
				statusField("operational", "operational", "maintenance", "down", "retired"),
				{Name: "meta", DisplayName: "Metadata", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", DefaultValue: map[string]any{}},
			},
		},
		{
			Name: "cmms_forms", DisplayName: "Forms", Icon: "clipboard-list", Color: "#8b5cf6",
			Fields: []FieldBlueprint{
				{Name: "name", DisplayName: "Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "code", DisplayName: "Code", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)", Required: true, Unique: true},
				{Name: "description", DisplayName: "Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
				{Name: "active", DisplayName: "Active", Type: collectiondomain.FieldTypeBoolean, DBType: "boolean", Required: true, DefaultValue: true},
				{Name: "current_version_id", DisplayName: "Current Version", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
			},
		},
		{
			Name: "cmms_form_versions", DisplayName: "Form Versions", Icon: "file-diff", Color: "#a855f7", Hidden: true,
			CompositeUnique: [][]string{{"form_id", "version"}},
			Fields: []FieldBlueprint{
				{Name: "form_id", DisplayName: "Form", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "version", DisplayName: "Version", Type: collectiondomain.FieldTypeInteger, DBType: "integer", Required: true, DefaultValue: 1},
				{Name: "schema", DisplayName: "Schema", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", Required: true, DefaultValue: map[string]any{}},
				{Name: "published", DisplayName: "Published", Type: collectiondomain.FieldTypeBoolean, DBType: "boolean", Required: true, DefaultValue: false},
			},
		},
		{
			Name: "cmms_operators", DisplayName: "Operators", Icon: "hard-hat", Color: "#f97316", Type: collectiondomain.CollectionTypeAuth,
			Fields: []FieldBlueprint{
				{Name: "email", DisplayName: "Email", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true, Unique: true},
				{Name: "first_name", DisplayName: "First Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "last_name", DisplayName: "Last Name", Type: collectiondomain.FieldTypeString, DBType: "varchar(100)"},
				{Name: "phone", DisplayName: "Phone", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)"},
			},
		},
		{
			Name: "cmms_tasks", DisplayName: "Tasks", Icon: "list-checks", Color: "#f59e0b",
			Fields: []FieldBlueprint{
				{Name: "title", DisplayName: "Title", Type: collectiondomain.FieldTypeString, DBType: "varchar(255)", Required: true},
				{Name: "number", DisplayName: "Number", Type: collectiondomain.FieldTypeString, DBType: "varchar(50)", Unique: true},
				{Name: "description", DisplayName: "Description", Type: collectiondomain.FieldTypeText, DBType: "text"},
				{Name: "asset_id", DisplayName: "Asset", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "form_id", DisplayName: "Form", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				// Pinned at task create so submissions stay valid when a new form version is published.
				{Name: "form_version_id", DisplayName: "Form Version", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "assigned_to", DisplayName: "Assigned Operator", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "assignee_user_id", DisplayName: "Assignee User", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				statusField("pending", "pending", "in_progress", "on_hold", "completed", "cancelled"),
				choiceField("priority", "Priority", "normal", "low", "normal", "high", "urgent"),
				{Name: "due_date", DisplayName: "Due Date", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
				{Name: "completed_at", DisplayName: "Completed At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
			},
		},
		{
			Name: "cmms_submissions", DisplayName: "Submissions", Icon: "file-check", Color: "#22c55e", Hidden: true,
			Fields: []FieldBlueprint{
				{Name: "task_id", DisplayName: "Task", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				// Must match the task's pinned form_version_id.
				{Name: "form_version_id", DisplayName: "Form Version", Type: collectiondomain.FieldTypeUUID, DBType: "uuid", Required: true},
				{Name: "operator_id", DisplayName: "Operator", Type: collectiondomain.FieldTypeUUID, DBType: "uuid"},
				{Name: "data", DisplayName: "Data", Type: collectiondomain.FieldTypeJSON, DBType: "jsonb", Required: true, DefaultValue: map[string]any{}},
				{Name: "submitted_at", DisplayName: "Submitted At", Type: collectiondomain.FieldTypeDateTime, DBType: "timestamptz"},
			},
		},
	}
}

func cmmsRelations() []RelationSpec {
	m2o := collectiondomain.RelationManyToOne
	return []RelationSpec{
		{CollectionMany: "cmms_form_versions", FieldMany: "form_id", CollectionOne: "cmms_forms", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "cmms_forms", FieldMany: "current_version_id", CollectionOne: "cmms_form_versions", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "cmms_tasks", FieldMany: "asset_id", CollectionOne: "cmms_assets", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "cmms_tasks", FieldMany: "form_id", CollectionOne: "cmms_forms", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "cmms_tasks", FieldMany: "form_version_id", CollectionOne: "cmms_form_versions", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "cmms_tasks", FieldMany: "assigned_to", CollectionOne: "cmms_operators", Type: m2o, OnDelete: "set_null"},
		{CollectionMany: "cmms_submissions", FieldMany: "task_id", CollectionOne: "cmms_tasks", Type: m2o, OnDelete: "cascade"},
		{CollectionMany: "cmms_submissions", FieldMany: "form_version_id", CollectionOne: "cmms_form_versions", Type: m2o, OnDelete: "restrict"},
		{CollectionMany: "cmms_submissions", FieldMany: "operator_id", CollectionOne: "cmms_operators", Type: m2o, OnDelete: "set_null"},
	}
}
