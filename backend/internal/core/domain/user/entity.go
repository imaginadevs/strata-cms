package user

import (
	"time"

	"github.com/google/uuid"

	domain "github.com/imaginacolombia/cms/internal/core/domain"
)

type AuthProvider string

const (
	ProviderLocal AuthProvider = "local"
	ProviderSSO   AuthProvider = "sso"
)

// Status represents the user account state.
type Status string

const (
	StatusActive    Status = "active"
	StatusInactive  Status = "inactive"
	StatusSuspended Status = "suspended"
)

type User struct {
	domain.Base

	Email        string       `json:"email"           db:"email"`
	FirstName    string       `json:"first_name"      db:"first_name"`
	LastName     string       `json:"last_name"       db:"last_name"`
	Avatar       *string      `json:"avatar"          db:"avatar"`
	Status       Status       `json:"status"          db:"status"`
	Provider     AuthProvider `json:"provider"        db:"provider"`
	ProviderUID  *string      `json:"provider_uid"    db:"provider_uid"`
	PasswordHash *string      `json:"-"               db:"password_hash"`
	LastLogin    *time.Time   `json:"last_login"      db:"last_login"`
	Language     string       `json:"language"        db:"language"`
	IsSuperAdmin bool         `json:"is_super_admin"  db:"is_super_admin"`
	Roles        []Role       `json:"roles,omitempty"`
}

type Role struct {
	domain.Base

	Name        string       `json:"name"        db:"name"`
	Description string       `json:"description" db:"description"`
	Icon        *string      `json:"icon"        db:"icon"`
	IsSystem    bool         `json:"is_system"   db:"is_system"`
	Permissions []Permission `json:"permissions,omitempty"`
}

type Permission struct {
	domain.Base

	RoleID     uuid.UUID `json:"role_id"     db:"role_id"`
	Collection string    `json:"collection"  db:"collection"`
	Action     Action    `json:"action"      db:"action"`
	Fields     []string  `json:"fields"      db:"fields"`
	Filter     any       `json:"filter"      db:"filter"`
}

type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
)

func New(tenantID uuid.UUID, email, firstName, lastName string, provider AuthProvider) *User {
	return &User{
		Base:      domain.NewBase(tenantID, nil),
		Email:     email,
		FirstName: firstName,
		LastName:  lastName,
		Status:    StatusActive,
		Provider:  provider,
		Language:  "en",
	}
}

func (u *User) FullName() string {
	return u.FirstName + " " + u.LastName
}

func (u *User) IsActive() bool {
	return u.Status == StatusActive
}
