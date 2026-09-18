package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/autogen"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// identRe mirrors the DB-layer safeIdent rule. Client-supplied field names
// (filter keys, sort fields) are validated against it before reaching the
// repository, where safeIdent would otherwise PANIC on an invalid identifier.
var identRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isSafeIdent(s string) bool { return identRe.MatchString(s) }

func MountItems(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &itemHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/items/{collection}", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, "", userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, "", userdomain.ActionCreate)).Post("/", h.create)
		r.With(httpMiddleware.RequirePermission(p, "", userdomain.ActionRead)).Get("/{id}", h.getByID)
		r.With(httpMiddleware.RequirePermission(p, "", userdomain.ActionUpdate)).Patch("/{id}", h.update)
		r.With(httpMiddleware.RequirePermission(p, "", userdomain.ActionDelete)).Delete("/{id}", h.delete)
	})
}

type itemHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

func itemFeatureForbidden(w http.ResponseWriter, r *http.Request, coll string) bool {
	settings := tenant.Settings{}
	if t, ok := httpMiddleware.TenantFromCtx(r.Context()); ok && t != nil {
		settings = t.Settings
	}
	if !features.CollectionAllowed(coll, settings) {
		writeError(w, http.StatusNotFound, "collection not found")
		return true
	}
	return false
}


func (h *itemHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	coll := chi.URLParam(r, "collection")
	if itemFeatureForbidden(w, r, coll) {
		return
	}
	page, limit := parsePage(r)

	filter := outbound.QueryFilter{Page: page, Limit: limit, Lang: r.URL.Query().Get("lang")}

	filters, ok := parseFilters(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid filter field name")
		return
	}
	filter.Filter = filters

	// Row-level permission filter (e.g. Customer role restricted to its own
	// orders) always wins over client-supplied filter values for the same field.
	if perm := h.resolvedPermFilter(r.Context(), coll); len(perm) > 0 {
		if filter.Filter == nil {
			filter.Filter = map[string]any{}
		}
		for k, v := range perm {
			filter.Filter[k] = v
		}
	}

	sort, ok := parseSort(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid sort field name")
		return
	}
	filter.Sort = sort

	items, total, err := h.repos.Item.List(r.Context(), tenantID, coll, filter)
	if err != nil {
		h.log.Errorw("items list failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	h.redactHashFields(r.Context(), tenantID, coll, items...)

	if deep := parseDeep(r); len(deep) > 0 {
		if err := h.expand(r.Context(), tenantID, coll, items, deep); err != nil {
			h.log.Errorw("items expand failed", "err", err, "collection", coll)
			writeError(w, http.StatusInternalServerError, "expand failed")
			return
		}
	}
	paginated(w, items, total, page, limit)
}

func (h *itemHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	coll := chi.URLParam(r, "collection")
	if itemFeatureForbidden(w, r, coll) {
		return
	}

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	actorID, hasActor := httpMiddleware.UserIDFromCtx(r.Context())
	var actorPtr *uuid.UUID
	if hasActor {
		actorPtr = &actorID
	}

	// Ownership enforcement: a permission filter like {"customer_id": {"_eq":
	// "$CURRENT_USER"}} forces that field to the caller's id, overriding
	// whatever the client sent, so an auth principal can never create a row
	// owned by someone else.
	if perm := h.resolvedPermFilter(r.Context(), coll); len(perm) > 0 {
		for k, v := range perm {
			data[k] = v
		}
	}

	if err := h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventBeforeCreate,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		Payload:    data,
	}); err != nil {
		writeBeforeHookError(w, err)
		return
	}

	h.applyAutoGen(r.Context(), tenantID, coll, data, true)
	h.hashFields(r.Context(), tenantID, coll, data)

	children := h.extractChildren(r.Context(), tenantID, coll, data)

	var item map[string]any
	var err error
	if len(children) > 0 {
		item, err = h.repos.Item.SaveGraph(r.Context(), tenantID, coll, "", data, children, true)
	} else {
		item, err = h.repos.Item.Create(r.Context(), tenantID, coll, data)
	}
	if err != nil {
		h.log.Errorw("item create failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	h.redactHashFields(r.Context(), tenantID, coll, item)

	var itemID *string
	if item != nil {
		if raw, ok := item["id"]; ok {
			s := fmt.Sprint(raw)
			itemID = &s
		}
	}
	_ = h.bus.PublishAfter(r.Context(), events.Event{
		Type:       events.EventAfterCreate,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		ItemID:     itemID,
		Payload:    item,
	})

	if deep := parseDeep(r); len(deep) > 0 && item != nil {
		_ = h.expand(r.Context(), tenantID, coll, []map[string]any{item}, deep)
	}
	writeJSON(w, http.StatusCreated, item)
}

// writeBeforeHookError translates a before-hook error into an HTTP response:
// errors.Is(err, events.ErrForbidden) → 403, everything else → 400.
func writeBeforeHookError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, events.ErrForbidden) {
		status = http.StatusForbidden
	}
	writeError(w, status, err.Error())
}

func (h *itemHandler) getByID(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	coll := chi.URLParam(r, "collection")
	if itemFeatureForbidden(w, r, coll) {
		return
	}
	id := chi.URLParam(r, "id")

	filter := outbound.QueryFilter{Lang: r.URL.Query().Get("lang")}
	if perm := h.resolvedPermFilter(r.Context(), coll); len(perm) > 0 {
		filter.Filter = perm
	}
	item, err := h.repos.Item.GetByID(r.Context(), tenantID, coll, id, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	h.redactHashFields(r.Context(), tenantID, coll, item)

	if deep := parseDeep(r); len(deep) > 0 {
		if err := h.expand(r.Context(), tenantID, coll, []map[string]any{item}, deep); err != nil {
			h.log.Errorw("item expand failed", "err", err, "collection", coll)
			writeError(w, http.StatusInternalServerError, "expand failed")
			return
		}
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *itemHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	coll := chi.URLParam(r, "collection")
	if itemFeatureForbidden(w, r, coll) {
		return
	}
	id := chi.URLParam(r, "id")
	actorID, hasActor := httpMiddleware.UserIDFromCtx(r.Context())
	var actorPtr *uuid.UUID
	if hasActor {
		actorPtr = &actorID
	}

	var data map[string]any
	if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	// Fetch with the row-level permission filter applied: a non-owner gets a
	// 404 here instead of updating (or even seeing) someone else's row.
	ownerFilter := outbound.QueryFilter{}
	if perm := h.resolvedPermFilter(r.Context(), coll); len(perm) > 0 {
		ownerFilter.Filter = perm
	}
	old, err := h.repos.Item.GetByID(r.Context(), tenantID, coll, id, ownerFilter)
	if err != nil {
		h.log.Errorw("item update preload failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if old == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	if err := h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventBeforeUpdate,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		ItemID:     &id,
		Old:        old,
		Payload:    data,
	}); err != nil {
		writeBeforeHookError(w, err)
		return
	}

	h.applyAutoGen(r.Context(), tenantID, coll, data, false)
	h.hashFields(r.Context(), tenantID, coll, data)

	children := h.extractChildren(r.Context(), tenantID, coll, data)

	var item map[string]any
	if len(children) > 0 {
		item, err = h.repos.Item.SaveGraph(r.Context(), tenantID, coll, id, data, children, false)
	} else {
		item, err = h.repos.Item.Update(r.Context(), tenantID, coll, id, data)
	}
	if err != nil {
		h.log.Errorw("item update failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if item == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	h.redactHashFields(r.Context(), tenantID, coll, item)

	itemID := id
	_ = h.bus.PublishAfter(r.Context(), events.Event{
		Type:       events.EventAfterUpdate,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		ItemID:     &itemID,
		Old:        old,
		Payload:    item,
		Meta:       map[string]any{"changed_fields": auditdomain.DiffFields(old, item)},
	})

	if deep := parseDeep(r); len(deep) > 0 {
		_ = h.expand(r.Context(), tenantID, coll, []map[string]any{item}, deep)
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *itemHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	coll := chi.URLParam(r, "collection")
	if itemFeatureForbidden(w, r, coll) {
		return
	}
	id := chi.URLParam(r, "id")
	actorID, hasActor := httpMiddleware.UserIDFromCtx(r.Context())
	var actorPtr *uuid.UUID
	if hasActor {
		actorPtr = &actorID
	}

	ownerFilter := outbound.QueryFilter{}
	if perm := h.resolvedPermFilter(r.Context(), coll); len(perm) > 0 {
		ownerFilter.Filter = perm
	}
	old, err := h.repos.Item.GetByID(r.Context(), tenantID, coll, id, ownerFilter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if old == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}

	if err := h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventBeforeDelete,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		ItemID:     &id,
		Old:        old,
	}); err != nil {
		writeBeforeHookError(w, err)
		return
	}

	if err := h.repos.Item.Delete(r.Context(), tenantID, coll, id); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}

	itemID := id
	_ = h.bus.PublishAfter(r.Context(), events.Event{
		Type:       events.EventAfterDelete,
		TenantID:   tenantID,
		Collection: coll,
		UserID:     actorPtr,
		ItemID:     &itemID,
		Old:        old,
	})

	w.WriteHeader(http.StatusNoContent)
}

// applyAutoGen inspects each field of the collection for an auto_generate option
// and mutates data in place before it is written to the database.
//
//   - isCreate=true  → applies fields with trigger "create" or "always"
//   - isCreate=false → applies fields with trigger "always" only
//
// Failures are non-fatal: they are logged and the field is left unchanged.
func (h *itemHandler) applyAutoGen(ctx context.Context, tenantID uuid.UUID, coll string, data map[string]any, isCreate bool) {
	schema, err := h.repos.Collection.GetByName(ctx, tenantID, coll)
	if err != nil || schema == nil {
		return
	}
	for _, field := range schema.AllFields() {
		if len(field.Options) == 0 {
			continue
		}
		ag, ok := field.Options["auto_generate"].(map[string]any)
		if !ok {
			continue
		}
		trigger := autogen.TriggerCreate
		if t, ok := ag["trigger"].(string); ok && t != "" {
			trigger = autogen.Trigger(t)
		}
		// On update, only "always" fields are recomputed.
		if !isCreate && trigger != autogen.TriggerAlways {
			continue
		}
		val, applied, err := autogen.ApplyField(field.Options, tenantID, coll, field.Name, data)
		if err != nil {
			h.log.Warnw("auto-gen failed", "field", field.Name, "err", err)
			continue
		}
		if applied {
			data[field.Name] = val
			_ = h.bus.Publish(ctx, events.Event{
				Type:       events.EventItemFieldAutoGenerated,
				TenantID:   tenantID,
				Collection: coll,
				Payload:    map[string]any{"field": field.Name, "fn": ag["fn"], "value": val},
			})
		}
	}
}

// ─── permission filter + hashing ───────────────────────────────────────────

// resolvedPermFilter reads the row-level filter attached by RequirePermission
// (from the matched Permission.Filter) and resolves its "$CURRENT_USER" /
// "$CURRENT_COLLECTION" tokens against the current request. Returns nil when
// no filter applies (e.g. Administrator bypass, or the permission has none).
//
// Supported filter shapes (Directus-style, restricted to what the underlying
// equality-only item query engine can express):
//
//	{"customer_id": "some-literal"}                  // direct equality
//	{"customer_id": {"_eq": "$CURRENT_USER"}}        // token substitution
func (h *itemHandler) resolvedPermFilter(ctx context.Context, coll string) map[string]any {
	raw := httpMiddleware.PermFilterFromCtx(ctx)
	if raw == nil {
		return nil
	}
	m, ok := asFilterMap(raw)
	if !ok || len(m) == 0 {
		return nil
	}
	userID, _ := httpMiddleware.UserIDFromCtx(ctx)
	out := make(map[string]any, len(m))
	for k, v := range m {
		val := v
		if sub, ok := v.(map[string]any); ok {
			if eq, ok := sub["_eq"]; ok {
				val = eq
			} else {
				continue // unsupported operator for the flat equality query engine
			}
		}
		out[k] = substituteFilterToken(val, userID, coll)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// asFilterMap normalises a Permission.Filter value (decoded from JSONB as
// map[string]any, or possibly re-marshalled through JSON as map[string]any
// with nested values) into a plain map, tolerating nil/empty ({}) filters.
func asFilterMap(raw any) (map[string]any, bool) {
	switch v := raw.(type) {
	case map[string]any:
		return v, true
	case nil:
		return nil, false
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			return nil, false
		}
		return m, true
	}
}

func substituteFilterToken(v any, userID uuid.UUID, coll string) any {
	if s, ok := v.(string); ok {
		switch s {
		case "$CURRENT_USER":
			return userID.String()
		case "$CURRENT_COLLECTION":
			return coll
		}
	}
	return v
}

// hashFields bcrypt-hashes every FieldTypeHash field present (and non-empty)
// in data, in place. Used for auth-collection fields like password_hash.
func (h *itemHandler) hashFields(ctx context.Context, tenantID uuid.UUID, coll string, data map[string]any) {
	for _, f := range h.hashFieldNames(ctx, tenantID, coll) {
		raw, ok := data[f]
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok || s == "" {
			delete(data, f) // never persist an empty/invalid hash value
			continue
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(s), bcrypt.DefaultCost)
		if err != nil {
			h.log.Warnw("hash field failed", "field", f, "collection", coll, "err", err)
			delete(data, f)
			continue
		}
		data[f] = string(hash)
	}
}

// redactHashFields strips FieldTypeHash fields (e.g. password_hash) from
// response payloads so hashes never leave the API.
func (h *itemHandler) redactHashFields(ctx context.Context, tenantID uuid.UUID, coll string, items ...map[string]any) {
	names := h.hashFieldNames(ctx, tenantID, coll)
	if len(names) == 0 {
		return
	}
	for _, item := range items {
		for _, f := range names {
			delete(item, f)
		}
	}
}

func (h *itemHandler) hashFieldNames(ctx context.Context, tenantID uuid.UUID, coll string) []string {
	schema, err := h.repos.Collection.GetByName(ctx, tenantID, coll)
	if err != nil || schema == nil {
		return nil
	}
	var out []string
	for _, f := range schema.AllFields() {
		if f.Type == collectiondomain.FieldTypeHash {
			out = append(out, f.Name)
		}
	}
	return out
}

// ─── relational helpers ─────────────────────────────────────────────────────

// parseFilters reads filter[<field>]=<value> query params into an equality map.
// Returns ok=false if any field name is not a safe identifier.
func parseFilters(r *http.Request) (map[string]any, bool) {
	var out map[string]any
	for key, vals := range r.URL.Query() {
		if !strings.HasPrefix(key, "filter[") || !strings.HasSuffix(key, "]") {
			continue
		}
		field := key[len("filter[") : len(key)-1]
		if !isSafeIdent(field) {
			return nil, false
		}
		if len(vals) == 0 {
			continue
		}
		if out == nil {
			out = map[string]any{}
		}
		out[field] = vals[0]
	}
	return out, true
}

// parseSort reads ?sort=field,-other into SortField list (leading '-' = desc).
// Returns ok=false if any field name is not a safe identifier.
func parseSort(r *http.Request) ([]outbound.SortField, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("sort"))
	if raw == "" {
		return nil, true
	}
	var out []outbound.SortField
	for _, tok := range strings.Split(raw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		desc := false
		if strings.HasPrefix(tok, "-") {
			desc = true
			tok = tok[1:]
		}
		if !isSafeIdent(tok) {
			return nil, false
		}
		out = append(out, outbound.SortField{Field: tok, Desc: desc})
	}
	return out, true
}

// parseDeep reads ?deep=relA,relB into a list of child-collection names to expand.
func parseDeep(r *http.Request) []string {
	raw := strings.TrimSpace(r.URL.Query().Get("deep"))
	if raw == "" {
		return nil
	}
	var out []string
	for _, tok := range strings.Split(raw, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// extractChildren pulls nested O2M child arrays out of the write payload, based on
// declared relations where coll is the parent (one) side. Each matched key is
// removed from data and returned as a ChildReconcile. Non-array values are ignored.
func (h *itemHandler) extractChildren(ctx context.Context, tenantID uuid.UUID, coll string, data map[string]any) []outbound.ChildReconcile {
	rels, err := h.repos.Relation.List(ctx, tenantID, coll)
	if err != nil || len(rels) == 0 {
		return nil
	}
	var out []outbound.ChildReconcile
	for _, rel := range rels {
		if rel.JunctionTable != nil || rel.CollectionOne != coll {
			continue // pure M2M and non-parent relations are not handled here
		}
		raw, ok := data[rel.CollectionMany]
		if !ok {
			continue
		}
		rows := toRowSlice(raw)
		if rows == nil {
			continue
		}
		delete(data, rel.CollectionMany)
		out = append(out, outbound.ChildReconcile{
			Collection: rel.CollectionMany,
			FKColumn:   rel.FieldMany,
			Owned:      strings.EqualFold(rel.OnDelete, "cascade"),
			Rows:       rows,
		})
	}
	return out
}

// expand attaches O2M children (named in deep) onto each parent item in place.
func (h *itemHandler) expand(ctx context.Context, tenantID uuid.UUID, coll string, items []map[string]any, deep []string) error {
	if len(items) == 0 || len(deep) == 0 {
		return nil
	}
	rels, err := h.repos.Relation.List(ctx, tenantID, coll)
	if err != nil {
		return err
	}
	want := map[string]bool{}
	for _, d := range deep {
		want[d] = true
	}
	parentIDs := make([]string, 0, len(items))
	for _, it := range items {
		parentIDs = append(parentIDs, toIDString(it["id"]))
	}

	for _, rel := range rels {
		if rel.JunctionTable != nil || rel.CollectionOne != coll || !want[rel.CollectionMany] {
			continue
		}
		var sort []outbound.SortField
		if h.childHasField(ctx, tenantID, rel.CollectionMany, "sort") {
			sort = []outbound.SortField{{Field: "sort"}}
		}
		kids, err := h.repos.Item.ChildrenByFK(ctx, tenantID, rel.CollectionMany, rel.FieldMany, parentIDs, sort)
		if err != nil {
			return err
		}
		group := map[string][]map[string]any{}
		for _, k := range kids {
			pid := toIDString(k[rel.FieldMany])
			group[pid] = append(group[pid], k)
		}
		for _, it := range items {
			g := group[toIDString(it["id"])]
			if g == nil {
				g = []map[string]any{}
			}
			it[rel.CollectionMany] = g
		}
	}
	return nil
}

// childHasField reports whether a collection declares a field with the given name.
func (h *itemHandler) childHasField(ctx context.Context, tenantID uuid.UUID, coll, field string) bool {
	c, err := h.repos.Collection.GetByName(ctx, tenantID, coll)
	if err != nil || c == nil {
		return false
	}
	for _, f := range c.AllFields() {
		if f.Name == field {
			return true
		}
	}
	return false
}

// toRowSlice converts a decoded JSON value into a slice of object rows, or nil if
// it is not an array of objects.
func toRowSlice(v any) []map[string]any {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, el := range arr {
		m, ok := el.(map[string]any)
		if !ok {
			return nil
		}
		out = append(out, m)
	}
	return out
}

// toIDString normalises an id value (across pgx decode forms) to a comparable string.
func toIDString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case [16]byte:
		return uuid.UUID(t).String()
	case pgtype.UUID:
		if !t.Valid {
			return ""
		}
		return uuid.UUID(t.Bytes).String()
	case fmt.Stringer:
		return t.String()
	default:
		return fmt.Sprint(v)
	}
}
