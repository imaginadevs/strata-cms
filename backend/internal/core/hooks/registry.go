package hooks

import (
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// Repos is the subset of tenant-scoped repositories a hook route may need.
type Repos struct {
	Item       outbound.ItemRepository
	Collection outbound.CollectionRepository
	User       outbound.UserRepository
	Role       outbound.RoleRepository
	Permission outbound.PermissionRepository
	Tenant     outbound.TenantRepository
}

// HookContext carries request-scoped tenant identity and repository access.
type HookContext struct {
	TenantID uuid.UUID
	Repos    Repos
}

// HookFunc handles one custom HTTP endpoint end-to-end.
type HookFunc func(w http.ResponseWriter, r *http.Request, hc HookContext)

// Route is one registered custom endpoint, mounted under /api/hooks/{Path}.
type Route struct {
	Method      string
	Path        string
	TenantScope *uuid.UUID // nil = any tenant; non-nil = only that tenant Host
	Fn          HookFunc
}

var (
	mu     sync.Mutex
	routes []Route
)

// RegisterRoute adds a custom HTTP endpoint under /api/hooks.
// tenantScope nil = available for every tenant Host; non-nil = only that tenant.
// Call from package init() and blank-import the package from router wiring.
func RegisterRoute(method, path string, tenantScope *uuid.UUID, fn HookFunc) {
	if fn == nil {
		panic("hooks: RegisterRoute requires a non-nil handler")
	}
	mu.Lock()
	defer mu.Unlock()
	routes = append(routes, Route{Method: method, Path: path, TenantScope: copyScope(tenantScope), Fn: fn})
}

// Routes returns a snapshot of every registered hook route.
func Routes() []Route {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Route, len(routes))
	copy(out, routes)
	return out
}

func copyScope(scope *uuid.UUID) *uuid.UUID {
	if scope == nil {
		return nil
	}
	c := *scope
	return &c
}

// Mounter is the minimal router capability Mount needs.
type Mounter interface {
	Get(pattern string, h http.HandlerFunc)
	Post(pattern string, h http.HandlerFunc)
	Put(pattern string, h http.HandlerFunc)
	Patch(pattern string, h http.HandlerFunc)
	Delete(pattern string, h http.HandlerFunc)
}

// TenantResolverFunc extracts the current tenant id from a request context.
type TenantResolverFunc func(r *http.Request) (uuid.UUID, bool)

// Mount registers every RegisterRoute onto r under "/hooks".
func Mount(r Mounter, resolve TenantResolverFunc, repos Repos) {
	for _, route := range Routes() {
		route := route
		handler := func(w http.ResponseWriter, req *http.Request) {
			tenantID, ok := resolve(req)
			if !ok {
				http.Error(w, `{"error":"tenant not resolved"}`, http.StatusUnauthorized)
				return
			}
			if route.TenantScope != nil && *route.TenantScope != tenantID {
				http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
				return
			}
			route.Fn(w, req, HookContext{TenantID: tenantID, Repos: repos})
		}
		path := "/hooks" + route.Path
		switch route.Method {
		case http.MethodGet:
			r.Get(path, handler)
		case http.MethodPost:
			r.Post(path, handler)
		case http.MethodPut:
			r.Put(path, handler)
		case http.MethodPatch:
			r.Patch(path, handler)
		case http.MethodDelete:
			r.Delete(path, handler)
		default:
			panic("hooks: unsupported method " + route.Method)
		}
	}
}
