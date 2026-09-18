package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	gql "github.com/graphql-go/graphql"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/tenant"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/features"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountGraphQL registers GET+POST /graphql.
// GET  → GraphiQL playground (HTML)
// POST → Execute a GraphQL query / mutation
func MountGraphQL(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := newGraphQLHandler(repos, bus, log)

	r.Get("/graphql", h.playground)
	r.Post("/graphql", h.execute)
}

// ─── Handler ──────────────────────────────────────────────────────────────────

type graphQLHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
	cache schemaCache
}

func newGraphQLHandler(repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) *graphQLHandler {
	h := &graphQLHandler{
		repos: repos,
		bus:   bus,
		log:   log,
		cache: schemaCache{schemas: make(map[uuid.UUID]*gql.Schema)},
	}

	// Invalidate the per-tenant schema cache whenever the schema changes.
	// This covers all six schema-mutation events.
	for _, evtType := range []events.EventType{
		events.EventCollectionCreated, events.EventCollectionUpdated, events.EventCollectionDeleted,
		events.EventFieldCreated, events.EventFieldUpdated, events.EventFieldDeleted,
	} {
		bus.Subscribe(evtType, func(_ context.Context, e events.Event) error {
			h.cache.invalidate(e.TenantID)
			return nil
		})
	}

	return h
}

// ─── Playground ───────────────────────────────────────────────────────────────

func (h *graphQLHandler) playground(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, graphiQLHTML)
}

// ─── Execute ──────────────────────────────────────────────────────────────────

type gqlRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables"`
	OperationName string         `json:"operationName"`
}

func (h *graphQLHandler) execute(w http.ResponseWriter, r *http.Request) {
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}

	schema, err := h.getSchema(r.Context(), tenantID)
	if err != nil {
		h.log.Errorw("graphql schema build failed", "tenant", tenantID, "err", err)
		writeError(w, http.StatusInternalServerError, "could not build schema")
		return
	}

	result := gql.Do(gql.Params{
		Schema:         *schema,
		RequestString:  req.Query,
		VariableValues: req.Variables,
		OperationName:  req.OperationName,
		Context:        r.Context(),
	})

	w.Header().Set("Content-Type", "application/json")
	if len(result.Errors) > 0 {
		w.WriteHeader(http.StatusOK) // GraphQL errors are still 200
	}
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

// ─── Schema cache ─────────────────────────────────────────────────────────────

type schemaCache struct {
	mu      sync.RWMutex
	schemas map[uuid.UUID]*gql.Schema
}

func (c *schemaCache) get(id uuid.UUID) (*gql.Schema, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c.schemas[id]
	return s, ok
}

func (c *schemaCache) set(id uuid.UUID, s *gql.Schema) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.schemas[id] = s
}

func (c *schemaCache) invalidate(id uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.schemas, id)
}

func (h *graphQLHandler) getSchema(ctx context.Context, tenantID uuid.UUID) (*gql.Schema, error) {
	if s, ok := h.cache.get(tenantID); ok {
		return s, nil
	}
	s, err := h.buildSchema(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	h.cache.set(tenantID, s)
	return s, nil
}

// ─── Schema builder ───────────────────────────────────────────────────────────

func (h *graphQLHandler) buildSchema(ctx context.Context, tenantID uuid.UUID) (*gql.Schema, error) {
	collections, err := h.repos.Collection.List(ctx, tenantID, false)
	if err != nil {
		return nil, fmt.Errorf("graphql: list collections: %w", err)
	}
	settings := tenant.Settings{}
	if t, err := h.repos.Tenant.GetByID(ctx, tenantID); err == nil && t != nil {
		settings = t.Settings
	}
	collections = features.FilterCollections(collections, settings)
	for i := range collections {
		full, err := h.repos.Collection.GetByName(ctx, tenantID, collections[i].Name)
		if err == nil && full != nil {
			collections[i] = full
		}
	}

	queryFields := gql.Fields{
		// Introspection works out of the box; add a ping field so the schema
		// is never completely empty (required by graphql-go).
		"_ping": {
			Type:        gql.String,
			Description: "Health check",
			Resolve:     func(_ gql.ResolveParams) (any, error) { return "pong", nil },
		},
	}
	mutationFields := gql.Fields{}

	for _, coll := range collections {
		objType := buildObjectType(coll)
		inputType := buildInputType(coll)
		resultType := buildResultType(coll.Name, objType)

		name := coll.Name
		pascal := toPascalCase(name)

		// ── Queries ──────────────────────────────────────────────────────
		repos := h.repos
		queryFields[name] = &gql.Field{
			Type:        resultType,
			Description: "List " + coll.DisplayName,
			Args: gql.FieldConfigArgument{
				"limit":  {Type: gql.Int, DefaultValue: 50},
				"page":   {Type: gql.Int, DefaultValue: 1},
				"search": {Type: gql.String},
			},
			Resolve: func(p gql.ResolveParams) (any, error) {
				limit, _ := p.Args["limit"].(int)
				page, _ := p.Args["page"].(int)
				filter := outbound.QueryFilter{Page: page, Limit: limit}
				if s, ok := p.Args["search"].(string); ok && s != "" {
					filter.Search = s
				}
				items, total, err := repos.Item.List(p.Context, tenantID, name, filter)
				if err != nil {
					return nil, err
				}
				return map[string]any{"data": normalizeItems(items), "total": total, "page": page, "limit": limit}, nil
			},
		}

		queryFields[name+"ByID"] = &gql.Field{
			Type:        objType,
			Description: "Get " + coll.DisplayName + " by ID",
			Args: gql.FieldConfigArgument{
				"id": {Type: gql.NewNonNull(gql.ID)},
			},
			Resolve: func(p gql.ResolveParams) (any, error) {
				id, _ := p.Args["id"].(string)
				item, err := repos.Item.GetByID(p.Context, tenantID, name, id, outbound.QueryFilter{})
				if err != nil {
					return nil, err
				}
				return normalizeItem(item), nil
			},
		}

		// ── Mutations ─────────────────────────────────────────────────────
		mutationFields["create"+pascal] = &gql.Field{
			Type:        objType,
			Description: "Create a " + coll.DisplayName,
			Args: gql.FieldConfigArgument{
				"data": {Type: gql.NewNonNull(inputType)},
			},
			Resolve: func(p gql.ResolveParams) (any, error) {
				data, _ := p.Args["data"].(map[string]any)
				item, err := repos.Item.Create(p.Context, tenantID, name, data)
				return normalizeItem(item), err
			},
		}

		mutationFields["update"+pascal] = &gql.Field{
			Type:        objType,
			Description: "Update a " + coll.DisplayName,
			Args: gql.FieldConfigArgument{
				"id":   {Type: gql.NewNonNull(gql.ID)},
				"data": {Type: gql.NewNonNull(inputType)},
			},
			Resolve: func(p gql.ResolveParams) (any, error) {
				id, _ := p.Args["id"].(string)
				data, _ := p.Args["data"].(map[string]any)
				item, err := repos.Item.Update(p.Context, tenantID, name, id, data)
				return normalizeItem(item), err
			},
		}

		mutationFields["delete"+pascal] = &gql.Field{
			Type:        gql.Boolean,
			Description: "Delete a " + coll.DisplayName,
			Args: gql.FieldConfigArgument{
				"id": {Type: gql.NewNonNull(gql.ID)},
			},
			Resolve: func(p gql.ResolveParams) (any, error) {
				id, _ := p.Args["id"].(string)
				err := repos.Item.Delete(p.Context, tenantID, name, id)
				return err == nil, err
			},
		}
	}

	queryType := gql.NewObject(gql.ObjectConfig{Name: "Query", Fields: queryFields})

	schemaConfig := gql.SchemaConfig{Query: queryType}
	if len(mutationFields) > 0 {
		schemaConfig.Mutation = gql.NewObject(gql.ObjectConfig{Name: "Mutation", Fields: mutationFields})
	}

	schema, err := gql.NewSchema(schemaConfig)
	if err != nil {
		return nil, fmt.Errorf("graphql: new schema: %w", err)
	}
	return &schema, nil
}

// ─── Type builders ────────────────────────────────────────────────────────────

// systemFields are included on every collection object type.
var systemFieldDefs = []struct {
	name, desc string
	t          gql.Output
}{
	{"id", "Unique identifier", gql.ID},
	{"created_at", "Creation timestamp (RFC3339)", gql.String},
	{"updated_at", "Last updated timestamp (RFC3339)", gql.String},
	{"created_by", "User ID who created this record", gql.ID},
	{"updated_by", "User ID who last updated this record", gql.ID},
}

func buildObjectType(coll *collectiondomain.Collection) *gql.Object {
	fields := gql.Fields{}
	for _, sf := range systemFieldDefs {
		sf := sf
		fields[sf.name] = &gql.Field{
			Type:        sf.t,
			Description: sf.desc,
			Resolve:     normalizeResolver(sf.name),
		}
	}
	for _, f := range coll.AllFields() {
		if f.IsSystem {
			continue
		}
		f := f
		fields[f.Name] = &gql.Field{
			Type:        gqlTypeFor(f.Type),
			Description: f.DisplayName,
			Resolve:     normalizeResolver(f.Name),
		}
	}
	return gql.NewObject(gql.ObjectConfig{
		Name:        toPascalCase(coll.Name) + "Type",
		Description: coll.DisplayName,
		Fields:      fields,
	})
}

func buildInputType(coll *collectiondomain.Collection) *gql.InputObject {
	fields := gql.InputObjectConfigFieldMap{}
	for _, f := range coll.AllFields() {
		if f.IsSystem {
			continue
		}
		fields[f.Name] = &gql.InputObjectFieldConfig{
			Type:        gqlInputTypeFor(f.Type, f.Required),
			Description: f.DisplayName,
		}
	}
	return gql.NewInputObject(gql.InputObjectConfig{
		Name:   toPascalCase(coll.Name) + "Input",
		Fields: fields,
	})
}

func buildResultType(collName string, objType *gql.Object) *gql.Object {
	return gql.NewObject(gql.ObjectConfig{
		Name: toPascalCase(collName) + "Result",
		Fields: gql.Fields{
			"data":  {Type: gql.NewList(objType)},
			"total": {Type: gql.Int},
			"page":  {Type: gql.Int},
			"limit": {Type: gql.Int},
		},
	})
}

// ─── Type mapping ─────────────────────────────────────────────────────────────

func gqlTypeFor(ft collectiondomain.FieldType) gql.Output {
	switch ft {
	case collectiondomain.FieldTypeInteger, collectiondomain.FieldTypeTimestamp:
		return gql.Int
	case collectiondomain.FieldTypeFloat, collectiondomain.FieldTypeDecimal:
		return gql.Float
	case collectiondomain.FieldTypeBoolean:
		return gql.Boolean
	case collectiondomain.FieldTypeUUID, collectiondomain.FieldTypeRelation:
		return gql.ID
	default:
		return gql.String
	}
}

func gqlInputTypeFor(ft collectiondomain.FieldType, required bool) gql.Input {
	var t gql.Input
	switch ft {
	case collectiondomain.FieldTypeInteger, collectiondomain.FieldTypeTimestamp:
		t = gql.Int
	case collectiondomain.FieldTypeFloat, collectiondomain.FieldTypeDecimal:
		t = gql.Float
	case collectiondomain.FieldTypeBoolean:
		t = gql.Boolean
	case collectiondomain.FieldTypeUUID, collectiondomain.FieldTypeRelation:
		t = gql.ID
	default:
		t = gql.String
	}
	if required {
		return gql.NewNonNull(t)
	}
	return t
}

// ─── Value normalization ──────────────────────────────────────────────────────

// normalizeResolver wraps the default map-key lookup with type coercion so
// pgx-native types (UUID bytes, time.Time) are serialized correctly.
func normalizeResolver(field string) gql.FieldResolveFn {
	return func(p gql.ResolveParams) (any, error) {
		m, ok := p.Source.(map[string]any)
		if !ok {
			return nil, nil
		}
		return coerce(m[field]), nil
	}
}

func coerce(v any) any {
	if v == nil {
		return nil
	}
	switch t := v.(type) {
	case [16]byte: // pgx UUID
		return uuid.UUID(t).String()
	case time.Time:
		return t.UTC().Format(time.RFC3339Nano)
	case []byte:
		return string(t)
	default:
		return v
	}
}

func normalizeItem(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	out := make(map[string]any, len(item))
	for k, v := range item {
		out[k] = coerce(v)
	}
	return out
}

func normalizeItems(items []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, normalizeItem(item))
	}
	return out
}

// ─── String helpers ───────────────────────────────────────────────────────────

// toPascalCase converts snake_case → PascalCase (posts → Posts, blog_posts → BlogPosts).
func toPascalCase(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if len(p) == 0 {
			continue
		}
		runes := []rune(p)
		b.WriteRune(unicode.ToUpper(runes[0]))
		for _, r := range runes[1:] {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ─── GraphiQL HTML ────────────────────────────────────────────────────────────

const graphiQLHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>CMS GraphQL Playground</title>
  <link rel="stylesheet" href="https://unpkg.com/graphiql@3/graphiql.min.css" />
  <style>
    body { margin: 0; height: 100vh; display: flex; flex-direction: column; }
    #graphiql { flex: 1; }
  </style>
</head>
<body>
  <div id="graphiql"></div>
  <script crossorigin src="https://unpkg.com/react@18/umd/react.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/react-dom@18/umd/react-dom.production.min.js"></script>
  <script crossorigin src="https://unpkg.com/graphiql@3/graphiql.min.js"></script>
  <script>
    const fetcher = GraphiQL.createFetcher({
      url: window.location.href,
      headers: { 'Content-Type': 'application/json' },
    });
    ReactDOM.createRoot(document.getElementById('graphiql')).render(
      React.createElement(GraphiQL, {
        fetcher,
        defaultEditorToolsVisibility: true,
      })
    );
  </script>
</body>
</html>`
