package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

// RequirePermission enforces RBAC for a fixed virtual/content collection + action.
// Pass collection="" to take the name from the "{collection}" chi URL param (items).
// Super-admins and users with the Administrator role bypass checks.
func RequirePermission(perms outbound.PermissionRepository, collection string, action userdomain.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authzBypass(r) {
				next.ServeHTTP(w, r)
				return
			}
			coll := collection
			if coll == "" {
				coll = chi.URLParam(r, "collection")
			}
			if coll == "" {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			if !checkPermission(r, perms, coll, action) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUsersMutate gates user create/update/delete/role/password admin ops.
// For PATCH/DELETE/password on /users/{id}: if the target is the caller, the
// request is allowed through so handlers can apply self-profile rules without
// requiring cms_users update. Admin operations on other users still need permission.
func RequireUsersMutate(perms outbound.PermissionRepository, action userdomain.Action) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authzBypass(r) {
				next.ServeHTTP(w, r)
				return
			}
			// Self access for PATCH/password only (not DELETE).
			if r.Method != http.MethodDelete {
				if idStr := chi.URLParam(r, "id"); idStr != "" {
					if target, err := uuid.Parse(idStr); err == nil {
						if caller, ok := UserIDFromCtx(r.Context()); ok && caller == target {
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}
			if !checkPermission(r, perms, userdomain.VirtualUsers, action) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func authzBypass(r *http.Request) bool {
	if IsSuperAdmin(r.Context()) || IsSuperAdminFromCtx(r.Context()) {
		return true
	}
	for _, role := range RolesFromCtx(r.Context()) {
		if role == "Administrator" {
			return true
		}
	}
	return false
}

// checkPermission resolves the applicable Permission for the current request and,
// if granted, stores its row-level Filter in the request context (CtxKeyPermFilter)
// for handlers to apply. Three identity paths are supported:
//
//   - No authenticated principal at all → treated as the built-in "Public" role.
//     This is how unauthenticated storefront/catalog reads are granted.
//   - Principal == "auth" (a row in a custom auth-type collection, e.g.
//     customers/operators) → resolved by the role names carried in the JWT,
//     since these principals have no row in cms_users / cms_roles.user_roles.
//   - Principal == "user" (or unset, for backward compatibility) → resolved by
//     user id via the standard cms_users → user_roles → permissions join.
func checkPermission(r *http.Request, perms outbound.PermissionRepository, collection string, action userdomain.Action) bool {
	ctx := r.Context()
	tenantID, ok := TenantIDFromCtx(ctx)
	if !ok {
		return false
	}

	userID, hasUser := UserIDFromCtx(ctx)
	principal := PrincipalFromCtx(ctx)

	var p *userdomain.Permission
	var err error
	switch {
	case !hasUser:
		p, err = perms.GetForRoleNames(ctx, tenantID, []string{"Public"}, collection, action)
	case principal == PrincipalAuth:
		roles := RolesFromCtx(ctx)
		if len(roles) == 0 {
			return false
		}
		p, err = perms.GetForRoleNames(ctx, tenantID, roles, collection, action)
	default:
		p, err = perms.GetForUser(ctx, tenantID, userID, collection, action)
	}
	if err != nil || p == nil {
		return false
	}

	*r = *r.WithContext(WithPermFilter(ctx, p.Filter))
	return true
}