package outbound

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
	"github.com/imaginacolombia/cms/internal/core/domain/audit"
	"github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/notification"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	"github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/domain/workflow"
)

// ─── Tenant ───────────────────────────────────────────────────────────────────

type TenantRepository interface {
	Create(ctx context.Context, t *tenant.Tenant) error
	GetByID(ctx context.Context, id uuid.UUID) (*tenant.Tenant, error)
	GetBySlug(ctx context.Context, slug string) (*tenant.Tenant, error)
	GetByDomain(ctx context.Context, domain string) (*tenant.Tenant, error)
	Update(ctx context.Context, t *tenant.Tenant) error
	List(ctx context.Context, filter TenantFilter) ([]*tenant.Tenant, int64, error)
	Delete(ctx context.Context, id uuid.UUID) error
	// AddStorageUsed adjusts the tenant's storage_used_bytes counter by delta
	// (negative to release). Floors at zero. Used for file upload/delete accounting.
	AddStorageUsed(ctx context.Context, id uuid.UUID, delta int64) error
}

type TenantFilter struct {
	Active *bool
	Page   int
	Limit  int
}

// ─── User ─────────────────────────────────────────────────────────────────────

type UserRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, u *user.User) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*user.User, error)
	GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*user.User, error)
	GetByProviderUID(ctx context.Context, tenantID uuid.UUID, provider user.AuthProvider, uid string) (*user.User, error)
	Update(ctx context.Context, tenantID uuid.UUID, u *user.User) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID, filter UserFilter) ([]*user.User, int64, error)
	SetPassword(ctx context.Context, tenantID, userID uuid.UUID, hash string) error
	RecordLogin(ctx context.Context, tenantID, userID uuid.UUID) error
	// ListAdminEmails returns active users who are super admins or hold the
	// Administrator role — used for admin-wide notifications.
	ListAdminEmails(ctx context.Context, tenantID uuid.UUID) ([]string, error)
	// SetInvite stores an invite token hash + expiry on a pending user.
	SetInvite(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	// GetByInviteToken finds a user by invite token hash (nil if none).
	GetByInviteToken(ctx context.Context, tenantID uuid.UUID, tokenHash string) (*user.User, error)
	// AcceptInvite sets the password, activates the account, and clears the invite.
	AcceptInvite(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) error
}

type UserFilter struct {
	Status *user.Status
	Search string
	RoleID *uuid.UUID
	Page   int
	Limit  int
	Sort   string
}

// ─── Role & Permission ────────────────────────────────────────────────────────

type RoleRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, r *user.Role) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*user.Role, error)
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*user.Role, error)
	Update(ctx context.Context, tenantID uuid.UUID, r *user.Role) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	List(ctx context.Context, tenantID uuid.UUID) ([]*user.Role, error)
	AssignToUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
	RevokeFromUser(ctx context.Context, tenantID, userID, roleID uuid.UUID) error
}

type PermissionRepository interface {
	Upsert(ctx context.Context, tenantID uuid.UUID, p *user.Permission) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	DeleteForRole(ctx context.Context, tenantID, roleID uuid.UUID) error
	ReplaceForRole(ctx context.Context, tenantID, roleID uuid.UUID, perms []*user.Permission) error
	GetForRole(ctx context.Context, tenantID, roleID uuid.UUID) ([]*user.Permission, error)
	GetForUser(ctx context.Context, tenantID, userID uuid.UUID, collection string, action user.Action) (*user.Permission, error)
	// GetForRoleNames resolves a permission by role name (e.g. "Public", or the
	// roles carried by an "auth" principal's JWT) instead of a user id. Used
	// for unauthenticated/public access and for principals from custom
	// auth-type collections that have no row in cms_users.
	GetForRoleNames(ctx context.Context, tenantID uuid.UUID, roleNames []string, collection string, action user.Action) (*user.Permission, error)
}

// ─── Notifications ────────────────────────────────────────────────────────────

type NotificationFilter struct {
	UnreadOnly bool
	Page       int
	Limit      int
}

type NotificationRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, n *notification.Notification) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*notification.Notification, error)
	ListForUser(ctx context.Context, tenantID, userID uuid.UUID, filter NotificationFilter) ([]*notification.Notification, int64, error)
	MarkRead(ctx context.Context, tenantID, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, tenantID, userID uuid.UUID) error
	CountUnread(ctx context.Context, tenantID, userID uuid.UUID) (int64, error)
}

// ─── Collection (model engine) ────────────────────────────────────────────────

type CollectionRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, c *collection.Collection) error
	GetByName(ctx context.Context, tenantID uuid.UUID, name string) (*collection.Collection, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*collection.Collection, error)
	Update(ctx context.Context, tenantID uuid.UUID, c *collection.Collection) error
	Delete(ctx context.Context, tenantID uuid.UUID, name string) error
	List(ctx context.Context, tenantID uuid.UUID, includeHidden bool) ([]*collection.Collection, error)
	// ReorderCollections applies group membership + sort to many collections at once.
	ReorderCollections(ctx context.Context, tenantID uuid.UUID, items []collection.CollectionPlacement) error

	// DDL operations — executed inside transactions against the live schema
	CreateTable(ctx context.Context, tenantID uuid.UUID, c *collection.Collection) error
	DropTable(ctx context.Context, tenantID uuid.UUID, name string) error
	RenameCollection(ctx context.Context, tenantID uuid.UUID, oldName, newName string) error
	AddColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collection.Field) error
	AlterColumn(ctx context.Context, tenantID uuid.UUID, collectionName string, f *collection.Field) error
	DropColumn(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error

	// Relation DDL — adds/removes FK columns and junction tables
	AddForeignKey(ctx context.Context, tenantID uuid.UUID, table, column, refTable string, unique, required bool, onDelete string) error
	CreateJunctionTable(ctx context.Context, tenantID uuid.UUID, junctionTable, manyTable, oneTable string) error

	CreateIndex(ctx context.Context, tenantID uuid.UUID, idx *collection.CollectionIndex) error
	DropIndex(ctx context.Context, tenantID uuid.UUID, indexName string) error
	SetColumnUnique(ctx context.Context, tenantID uuid.UUID, tableName, columnName string, add bool) error
	AddCompositeUnique(ctx context.Context, tenantID uuid.UUID, tableName string, columns []string) error
}

type CollectionGroupRepository interface {
	List(ctx context.Context, tenantID uuid.UUID) ([]*collection.CollectionGroup, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*collection.CollectionGroup, error)
	Create(ctx context.Context, tenantID uuid.UUID, g *collection.CollectionGroup) error
	Update(ctx context.Context, tenantID uuid.UUID, g *collection.CollectionGroup) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	Reorder(ctx context.Context, tenantID uuid.UUID, orderedIDs []uuid.UUID) error
}

// APITokenRepository persists policy-scoped programmatic access tokens.
type APITokenRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, t *apitoken.APIToken) error
	List(ctx context.Context, tenantID uuid.UUID) ([]*apitoken.APIToken, error)
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*apitoken.APIToken, error)
	GetByPrefix(ctx context.Context, tenantID uuid.UUID, prefix string) (*apitoken.APIToken, error)
	Update(ctx context.Context, tenantID uuid.UUID, t *apitoken.APIToken) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	TouchLastUsed(ctx context.Context, tenantID, id uuid.UUID, at time.Time) error
}

type SectionRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, s *collection.Section) error
	GetByName(ctx context.Context, tenantID uuid.UUID, collectionName, sectionName string) (*collection.Section, error)
	Update(ctx context.Context, tenantID uuid.UUID, s *collection.Section) error
	Delete(ctx context.Context, tenantID uuid.UUID, collectionName, sectionName string) error
	ListByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collection.Section, error)
	CountByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) (int, error)
	ReassignFields(ctx context.Context, tenantID, fromSectionID, toSectionID uuid.UUID) error
}

type FieldRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, f *collection.Field) error
	GetByName(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) (*collection.Field, error)
	Update(ctx context.Context, tenantID uuid.UUID, f *collection.Field) error
	Delete(ctx context.Context, tenantID uuid.UUID, collectionName, fieldName string) error
	ListByCollection(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collection.Field, error)
}

type RelationRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, r *collection.Relation) error
	List(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collection.Relation, error)
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
}

// ─── Items (dynamic data) ─────────────────────────────────────────────────────

// QueryFilter is a generic filter passed from REST/GraphQL to the item adapter.
type QueryFilter struct {
	Filter map[string]any
	Sort   []SortField
	Fields []string
	Limit  int
	Page   int
	Search string
	Lang   string // active language for i18n fields
}

type SortField struct {
	Field string
	Desc  bool
}

type ItemRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, collection string, data map[string]any) (map[string]any, error)
	GetByID(ctx context.Context, tenantID uuid.UUID, collection, id string, filter QueryFilter) (map[string]any, error)
	List(ctx context.Context, tenantID uuid.UUID, collection string, filter QueryFilter) ([]map[string]any, int64, error)
	Update(ctx context.Context, tenantID uuid.UUID, collection, id string, data map[string]any) (map[string]any, error)
	Delete(ctx context.Context, tenantID uuid.UUID, collection, id string) error

	// ChildrenByFK returns child rows whose fkColumn matches any of parentIDs,
	// ordered by sort (defaults to created_at). Used to expand O2M relations on
	// read without N+1 (single query per relation via = ANY).
	ChildrenByFK(ctx context.Context, tenantID uuid.UUID, collection, fkColumn string, parentIDs []string, sort []SortField) ([]map[string]any, error)

	// SaveGraph upserts a parent row and reconciles its nested child relations in
	// a single transaction. isCreate selects INSERT vs UPDATE for the parent.
	// Returns the parent row (nil if updating a non-existent id).
	SaveGraph(ctx context.Context, tenantID uuid.UUID, collection, id string, parent map[string]any, children []ChildReconcile, isCreate bool) (map[string]any, error)
}

// ChildReconcile describes the desired state of one O2M child relation during a
// nested write. Rows is the full desired set (each row may carry an "id"): rows
// with an id are updated, rows without are inserted. When Owned is true, existing
// child rows omitted from Rows are deleted; otherwise they are left untouched.
type ChildReconcile struct {
	Collection string           // child collection (holds the FK column)
	FKColumn   string           // FK column on the child pointing at the parent
	Owned      bool             // delete children omitted from Rows (composition)
	Rows       []map[string]any // desired child rows
}

// ─── Audit ────────────────────────────────────────────────────────────────────

type AuditRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, log *audit.Log) error
	List(ctx context.Context, tenantID uuid.UUID, filter AuditFilter) ([]*audit.Log, int64, error)
}

type AuditFilter struct {
	Collection *string
	UserID     *uuid.UUID
	Operation  *audit.Operation
	Page       int
	Limit      int
}

// IndexRepository stores metadata for user-defined indexes.
type IndexRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, idx *collection.CollectionIndex) error
	List(ctx context.Context, tenantID uuid.UUID, collectionName string) ([]*collection.CollectionIndex, error)
	GetByID(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) (*collection.CollectionIndex, error)
	Delete(ctx context.Context, tenantID uuid.UUID, id uuid.UUID) error
}

// ─── Session / Token ─────────────────────────────────────────────────────────

type SessionRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, s *Session) error
	GetByToken(ctx context.Context, tenantID uuid.UUID, token string) (*Session, error)
	Delete(ctx context.Context, tenantID uuid.UUID, token string) error
	DeleteAllForUser(ctx context.Context, tenantID, userID uuid.UUID) error
}

// Session represents an issued refresh-token session. UserID is set for
// system-user (cms_users) sessions; PrincipalCollection/PrincipalID identify
// a row in a custom auth-type collection (e.g. customers, operators) instead.
// Exactly one of the two identity forms is populated.
type Session struct {
	ID                  uuid.UUID
	UserID              *uuid.UUID
	PrincipalCollection *string
	PrincipalID         *uuid.UUID
	RefreshToken        string
	ExpiresAt           interface{}
	IPAddress           *string
	UserAgent           *string
}

// ─── Workflows ────────────────────────────────────────────────────────────────

type WorkflowFilter struct {
	Active      *bool
	TriggerType *workflow.TriggerType
	Page        int
	Limit       int
}

type WorkflowRunFilter struct {
	WorkflowID *uuid.UUID
	Status     *workflow.RunStatus
	Page       int
	Limit      int
}

type WorkflowRepository interface {
	Create(ctx context.Context, tenantID uuid.UUID, wf *workflow.Workflow) error
	Update(ctx context.Context, tenantID uuid.UUID, wf *workflow.Workflow) error
	Delete(ctx context.Context, tenantID, id uuid.UUID) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (*workflow.Workflow, error)
	List(ctx context.Context, tenantID uuid.UUID, filter WorkflowFilter) ([]*workflow.Workflow, int64, error)
	ListActiveByTrigger(ctx context.Context, tenantID uuid.UUID, triggerType workflow.TriggerType) ([]*workflow.Workflow, error)
	SetLastFiredAt(ctx context.Context, tenantID, id uuid.UUID, at time.Time) error
	// CountByOrigin counts workflows provisioned by a given plugin (for idempotent install).
	CountByOrigin(ctx context.Context, tenantID uuid.UUID, originPlugin string) (int, error)

	CreateRun(ctx context.Context, tenantID uuid.UUID, run *workflow.Run) error
	UpdateRun(ctx context.Context, tenantID uuid.UUID, run *workflow.Run) error
	UpdateRunStep(ctx context.Context, tenantID uuid.UUID, step *workflow.RunStep) error
	GetRunByID(ctx context.Context, tenantID, id uuid.UUID) (*workflow.Run, error)
	ListRuns(ctx context.Context, tenantID uuid.UUID, filter WorkflowRunFilter) ([]*workflow.Run, int64, error)
	HasActiveRun(ctx context.Context, tenantID, workflowID uuid.UUID) (bool, error)
}
