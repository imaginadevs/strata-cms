package audit

import (
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/ids"
)

// Operation represents what happened to the record.
type Operation string

const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
	OperationLogin  Operation = "login"
	OperationLogout Operation = "logout"
)

// Log records every mutation made by a user within a tenant.
// Schema: user __ did __ on collection/item, old data __, new data __
type Log struct {
	ID            uuid.UUID      `json:"id"            db:"id"`
	UserID        *uuid.UUID     `json:"user_id"       db:"user_id"`
	UserEmail     *string        `json:"user_email"    db:"user_email"`
	Collection    string         `json:"collection"    db:"collection"`
	ItemID        *string        `json:"item_id"       db:"item_id"`
	Operation     Operation      `json:"operation"     db:"operation"`
	OldData       map[string]any `json:"old_data"      db:"old_data"`
	NewData       map[string]any `json:"new_data"      db:"new_data"`
	ChangedFields []string       `json:"changed_fields" db:"changed_fields"`
	IPAddress     *string        `json:"ip_address"    db:"ip_address"`
	UserAgent     *string        `json:"user_agent"    db:"user_agent"`
	CreatedAt     time.Time      `json:"created_at"    db:"created_at"`
}

func New(
	tenantID uuid.UUID,
	userID *uuid.UUID,
	userEmail *string,
	collection string,
	itemID *string,
	op Operation,
	oldData, newData map[string]any,
) *Log {
	_ = tenantID
	return &Log{
		ID:            ids.New(),
		UserID:        userID,
		UserEmail:     userEmail,
		Collection:    collection,
		ItemID:        itemID,
		Operation:     op,
		OldData:       oldData,
		NewData:       newData,
		ChangedFields: []string{}, // never nil — column is NOT NULL
		CreatedAt:     time.Now().UTC(),
	}
}

// DiffFields returns the field names that changed between old and new data.
func DiffFields(old, new map[string]any) []string {
	changed := make([]string, 0)
	for k, newVal := range new {
		// reflect.DeepEqual handles nested maps/slices/pointers safely; a plain
		// != would panic on uncomparable values (e.g. the settings sub-object).
		if oldVal, exists := old[k]; !exists || !reflect.DeepEqual(oldVal, newVal) {
			changed = append(changed, k)
		}
	}
	return changed
}
