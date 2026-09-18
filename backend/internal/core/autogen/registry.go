// Package autogen provides an extensible registry of field auto-generation
// functions. Functions can be registered globally (all tenants) or scoped to a
// specific tenant so different deployments can add their own transforms without
// touching core code.
//
// Registering a global function:
//
//	autogen.Register(autogen.FunctionDef{ID: "my_fn", ...}, myFn)
//
// Registering a tenant-specific function at startup:
//
//	autogen.RegisterForTenant(tenantID, autogen.FunctionDef{ID: "brand_slug", ...}, brandSlugFn)
//
// A tenant-specific function with the same ID as a global one overrides it for
// that tenant only.
package autogen

import (
	"sync"

	"github.com/google/uuid"
)

// Trigger controls when the auto-generation function fires.
type Trigger string

const (
	// TriggerCreate fires only when a new item is created.
	TriggerCreate Trigger = "create"
	// TriggerAlways fires on both create and update.
	TriggerAlways Trigger = "always"
)

// Context is passed to every AutoGenFn invocation.
type Context struct {
	TenantID    uuid.UUID
	Collection  string
	FieldName   string
	SourceField string         // value of options["source_field"]
	Options     map[string]any // full auto_generate sub-object from field options
	Item        map[string]any // item data being saved (may already contain the field)
}

// AutoGenFn receives a Context and returns the computed value.
// Return (nil, nil) to leave the field unchanged.
type AutoGenFn func(ctx Context) (any, error)

// FunctionDef is the descriptor returned by the API and consumed by the frontend.
type FunctionDef struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	NeedsSource bool   `json:"needs_source_field"` // whether source_field must be set
}

type entry struct {
	def FunctionDef
	fn  AutoGenFn
}

// Registry holds global and per-tenant auto-generation functions.
type Registry struct {
	mu      sync.RWMutex
	global  map[string]entry
	tenants map[uuid.UUID]map[string]entry
}

// Default is the application-wide registry.  Built-in functions register
// themselves here via init() in builtins.go.
var Default = &Registry{
	global:  make(map[string]entry),
	tenants: make(map[uuid.UUID]map[string]entry),
}

// ─── Package-level convenience wrappers ──────────────────────────────────────

// Register adds fn as a globally available function (available to all tenants).
func Register(def FunctionDef, fn AutoGenFn) { Default.Register(def, fn) }

// RegisterForTenant adds fn visible only for the given tenant.
// A tenant-specific function with the same ID as a global one overrides it for
// that tenant.
func RegisterForTenant(tenantID uuid.UUID, def FunctionDef, fn AutoGenFn) {
	Default.RegisterForTenant(tenantID, def, fn)
}

// List returns all functions available for tenantID (global + tenant-specific,
// tenant functions take precedence).
func List(tenantID uuid.UUID) []FunctionDef { return Default.List(tenantID) }

// ApplyField executes the auto-generation configured in fieldOptions for a
// single field.  Returns (value, true, nil) when a value was produced,
// (nil, false, nil) when no auto_generate config exists or the function is
// unknown.
func ApplyField(fieldOptions map[string]any, tenantID uuid.UUID, collection, fieldName string, item map[string]any) (any, bool, error) {
	return Default.ApplyField(fieldOptions, tenantID, collection, fieldName, item)
}

// ─── Registry methods ─────────────────────────────────────────────────────────

func (r *Registry) Register(def FunctionDef, fn AutoGenFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.global[def.ID] = entry{def: def, fn: fn}
}

func (r *Registry) RegisterForTenant(tenantID uuid.UUID, def FunctionDef, fn AutoGenFn) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tenants[tenantID] == nil {
		r.tenants[tenantID] = make(map[string]entry)
	}
	r.tenants[tenantID][def.ID] = entry{def: def, fn: fn}
}

func (r *Registry) List(tenantID uuid.UUID) []FunctionDef {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]FunctionDef, 0, len(r.global))
	seen := make(map[string]struct{})

	// Tenant overrides first.
	if m, ok := r.tenants[tenantID]; ok {
		for id, e := range m {
			out = append(out, e.def)
			seen[id] = struct{}{}
		}
	}
	for id, e := range r.global {
		if _, exists := seen[id]; !exists {
			out = append(out, e.def)
		}
	}
	return out
}

func (r *Registry) get(tenantID uuid.UUID, fnID string) (AutoGenFn, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.tenants[tenantID]; ok {
		if e, ok := m[fnID]; ok {
			return e.fn, true
		}
	}
	if e, ok := r.global[fnID]; ok {
		return e.fn, true
	}
	return nil, false
}

// ApplyField is the method form of the package-level helper.
func (r *Registry) ApplyField(fieldOptions map[string]any, tenantID uuid.UUID, collection, fieldName string, item map[string]any) (any, bool, error) {
	raw, ok := fieldOptions["auto_generate"]
	if !ok {
		return nil, false, nil
	}
	cfg, ok := raw.(map[string]any)
	if !ok {
		return nil, false, nil
	}
	fnID, _ := cfg["fn"].(string)
	if fnID == "" {
		return nil, false, nil
	}
	fn, ok := r.get(tenantID, fnID)
	if !ok {
		return nil, false, nil
	}
	sourceField, _ := cfg["source_field"].(string)
	val, err := fn(Context{
		TenantID:    tenantID,
		Collection:  collection,
		FieldName:   fieldName,
		SourceField: sourceField,
		Options:     cfg,
		Item:        item,
	})
	if err != nil {
		return nil, false, err
	}
	return val, true, nil
}
