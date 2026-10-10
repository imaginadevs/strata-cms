package postgres

import (
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/core/email"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// tenantSchema returns the Postgres schema name for a tenant's data tables.
// The schema name is the full tenant UUID (e.g. "00000000-0000-0000-0000-000000000001").
// When used in SQL always wrap with schemaOf() or quotedIdent() to produce a quoted identifier.
func tenantSchema(id uuid.UUID) string {
	return id.String()
}

// quotedIdent double-quotes a Postgres identifier, escaping any embedded
// double-quotes by doubling them. Safe for identifiers that contain hyphens
// (e.g. UUID strings used as schema names).
func quotedIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// schemaOf returns the quoted schema identifier for a tenant, ready for use
// in fmt.Sprintf SQL:
//
//	fmt.Sprintf("SELECT * FROM %s.users", schemaOf(tenantID))
func schemaOf(id uuid.UUID) string {
	return quotedIdent(tenantSchema(id))
}

// Repos bundles all repository implementations backed by a single pgxpool.
type Repos struct {
	Tenant          outbound.TenantRepository
	User            outbound.UserRepository
	Session         outbound.SessionRepository
	Role            outbound.RoleRepository
	Permission      outbound.PermissionRepository
	Collection      outbound.CollectionRepository
	CollectionGroup outbound.CollectionGroupRepository
	Field           outbound.FieldRepository
	Section         outbound.SectionRepository
	Relation        outbound.RelationRepository
	Item            outbound.ItemRepository
	Audit           outbound.AuditRepository
	Index           outbound.IndexRepository
	Notification    outbound.NotificationRepository
	Workflow        outbound.WorkflowRepository
	File            outbound.FileRepository
	APIToken        outbound.APITokenRepository
	EmailTemplate   email.TemplateStore
}

// NewRepos wires all repository implementations against the provided pool.
func NewRepos(db *pgxpool.Pool) *Repos {
	ddl := newDDLAdapter(db)
	return &Repos{
		Tenant:          &tenantRepository{db: db},
		User:            &userRepository{db: db},
		Session:         &sessionRepository{db: db},
		Role:            &roleRepository{db: db},
		Permission:      &permissionRepository{db: db},
		Collection:      &collectionRepository{db: db, ddl: ddl},
		CollectionGroup: &collectionGroupRepository{db: db},
		Field:           &fieldRepository{db: db},
		Section:         &sectionRepository{db: db},
		Relation:        &relationRepository{db: db},
		Item:            &itemRepository{db: db},
		Audit:           &auditRepository{db: db},
		Index:           &indexRepository{db: db},
		Notification:    &notificationRepository{db: db},
		Workflow:        &workflowRepository{db: db},
		File:            &fileRepository{db: db},
		APIToken:        &apiTokenRepository{db: db},
		EmailTemplate:   &emailTemplateRepository{db: db},
	}
}
