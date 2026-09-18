package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type superAdminKey struct{}

// SuperAdminGate is middleware that:
//  1. Marks the request as coming from a super-admin if the JWT claims it.
//  2. Enforces that super-admin requests originate from the IP whitelist.
//     The whitelist is loaded from config only (never from the database),
//     so it cannot be escalated by any tenant or user action.
//  3. Blocks the request with 403 if the IP is not in the whitelist.
func SuperAdminGate(allowedIPs []string) func(http.Handler) http.Handler {
	cidrs := parseCIDRs(allowedIPs)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This middleware only acts when the JWT includes a super-admin claim.
			// If no claim is set, pass through unchanged (regular auth applies).
			if !isSuperAdminClaim(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}

			// IP enforcement — super admins MUST come from an allowed IP.
			clientIP := extractIP(r)
			if !isAllowed(clientIP, cidrs) {
				http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
				return
			}

			// Mark context so handlers know this is a super-admin request.
			ctx := context.WithValue(r.Context(), superAdminKey{}, true)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IsSuperAdmin returns true if the current request is authenticated as super-admin
// AND has passed the IP check.
func IsSuperAdmin(ctx context.Context) bool {
	v, _ := ctx.Value(superAdminKey{}).(bool)
	return v
}

// isSuperAdminClaim reads the JWT roles to check for the super-admin marker.
// The super-admin role name is intentionally not documented in public API docs.
func isSuperAdminClaim(ctx context.Context) bool {
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
		// Support plain IPs as well as CIDR notation
		if !strings.Contains(s, "/") {
			s += "/32"
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
