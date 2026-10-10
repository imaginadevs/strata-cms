package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type superAdminKey struct{}

// SuperAdminGate gates the USE of super-admin privilege by source IP — it does
// NOT restrict ordinary authenticated access.
//
//   - Request has no super-admin claim → passed through unchanged.
//   - Super-admin claim from an allowed IP → elevated: the request is marked
//     super-admin and super-admin-only handlers accept it.
//   - Super-admin claim from any other IP → NOT elevated, but still allowed to
//     proceed as a normal tenant user. Super-admin-only handlers (which check
//     IsSuperAdmin) will refuse; everything else works as it would for any admin.
//
// This matters because a super-admin user still operates their own tenant like a
// normal admin, so the IP allowlist must not 403 them out of regular endpoints.
// The whitelist is loaded from config only (never from the database), so it cannot
// be escalated by any tenant or user action.
func SuperAdminGate(allowedIPs []string) func(http.Handler) http.Handler {
	cidrs := parseCIDRs(allowedIPs)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isSuperAdminClaim(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			// Elevate to super-admin only from an allowed IP; otherwise proceed
			// without elevation (no 403 here — that's for the handlers to decide).
			if isAllowed(extractIP(r), cidrs) {
				ctx := context.WithValue(r.Context(), superAdminKey{}, true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// IsSuperAdmin returns true if the current request is authenticated as super-admin
// AND has passed the IP check.
func IsSuperAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(superAdminKey{}).(bool)
	return v
}

// isSuperAdminClaim reports whether the authenticated principal is a super admin.
// The authoritative source is the token's `sa` claim (set from users.is_super_admin
// and injected as CtxKeyIsSuperAdmin by Authenticate). A legacy "super_admin" role
// name is also honored for backward compatibility.
func isSuperAdminClaim(ctx context.Context) bool {
	if IsSuperAdminFromCtx(ctx) {
		return true
	}
	roles := RolesFromCtx(ctx)
	for _, r := range roles {
		if r == "super_admin" {
			return true
		}
	}
	return false
}

// ─── IP helpers ───────────────────────────────────────────────────────────────

func parseCIDRs(list []string) []*net.IPNet {
	nets := make([]*net.IPNet, 0, len(list))
	for _, s := range list {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		// Support plain IPs as well as CIDR notation. A bare IPv6 address needs a
		// /128 host mask (not /32, which would widen it to a huge range).
		if !strings.Contains(s, "/") {
			if strings.Contains(s, ":") {
				s += "/128"
			} else {
				s += "/32"
			}
		}
		_, ipNet, err := net.ParseCIDR(s)
		if err == nil {
			nets = append(nets, ipNet)
		}
	}
	return nets
}

func isAllowed(ip string, cidrs []*net.IPNet) bool {
	if len(cidrs) == 0 {
		return false // no whitelist → deny all super-admin access
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range cidrs {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}

func extractIP(r *http.Request) string {
	// Prefer X-Real-IP set by a trusted reverse proxy.
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	// Then X-Forwarded-For (first address is the client).
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.SplitN(fwd, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
