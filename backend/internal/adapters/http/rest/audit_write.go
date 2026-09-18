package rest

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/google/uuid"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// recordAudit writes an audit-log entry. It is best-effort: any failure is logged
// but never propagated, so auditing can never break the request it records.
// When both oldData and newData are supplied, changed_fields is computed.
func recordAudit(
	ctx context.Context,
	repos *postgresrepo.Repos,
	log *logger.Logger,
	r *http.Request,
	tenantID uuid.UUID,
	actorID *uuid.UUID,
	email *string,
	collection string,
	itemID *string,
	op auditdomain.Operation,
	oldData, newData map[string]any,
) {
	entry := auditdomain.New(tenantID, actorID, email, collection, itemID, op, oldData, newData)
	if oldData != nil && newData != nil {
		entry.ChangedFields = auditdomain.DiffFields(oldData, newData)
	}
	if ip := clientIP(r); ip != "" {
		entry.IPAddress = &ip
	}
	if ua := r.UserAgent(); ua != "" {
		entry.UserAgent = &ua
	}
	if err := repos.Audit.Create(ctx, tenantID, entry); err != nil {
		log.Errorw("audit write failed", "err", err, "op", op, "collection", collection)
	}
}

// recordAuditCtx records an entry for a protected route, pulling actor id and
// email from the request context (set by the auth middleware).
func recordAuditCtx(
	ctx context.Context,
	repos *postgresrepo.Repos,
	log *logger.Logger,
	r *http.Request,
	collection string,
	itemID *string,
	op auditdomain.Operation,
	oldData, newData map[string]any,
) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(ctx)
	var actor *uuid.UUID
	if uid, ok := httpMiddleware.UserIDFromCtx(ctx); ok {
		actor = &uid
	}
	var email *string
	if e := httpMiddleware.EmailFromCtx(ctx); e != "" {
		email = &e
	}
	recordAudit(ctx, repos, log, r, tenantID, actor, email, collection, itemID, op, oldData, newData)
}

// clientIP returns a bare IP (no port) suitable for the audit_logs.inet column.
func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// deref returns the pointed-to string, or nil — so audit snapshots store values
// (comparable by DiffFields), not pointers.
func deref(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// toMap converts a value to a generic map via JSON, for nested audit snapshots.
func toMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil
	}
	return m
}
