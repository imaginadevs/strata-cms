package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/email"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountEmailTemplates exposes the registered default templates and per-tenant
// overrides. Overrides are validated on save; at send time a broken override
// falls back to the built-in default (see core/email).
func MountEmailTemplates(r chi.Router, repos *postgresrepo.Repos, reg *email.Registry, log *logger.Logger) {
	h := &emailTemplateHandler{store: repos.EmailTemplate, reg: reg, log: log}
	p := repos.Permission
	r.Route("/email-templates", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/", h.list)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionRead)).Get("/{key}", h.get)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionUpdate)).Put("/{key}", h.upsert)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSettings, userdomain.ActionDelete)).Delete("/{key}", h.delete)
	})
}

type emailTemplateHandler struct {
	store email.TemplateStore
	reg   *email.Registry
	log   *logger.Logger
}

func (h *emailTemplateHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	overrides := map[string]email.StoredTemplate{}
	if list, err := h.store.List(r.Context(), tenantID); err == nil {
		for _, st := range list {
			overrides[st.Key] = st
		}
	}
	type row struct {
		Key        string `json:"key"`
		Subject    string `json:"default_subject"`
		Overridden bool   `json:"overridden"`
		Enabled    bool   `json:"enabled"`
	}
	out := make([]row, 0)
	for _, key := range h.reg.Keys() {
		def, _ := h.reg.Get(key)
		ov, has := overrides[key]
		out = append(out, row{Key: key, Subject: def.Subject, Overridden: has, Enabled: !has || ov.Enabled})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *emailTemplateHandler) get(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	key := chi.URLParam(r, "key")
	def, ok := h.reg.Get(key)
	if !ok {
		writeError(w, http.StatusNotFound, "unknown template")
		return
	}
	var override *email.StoredTemplate
	if list, err := h.store.List(r.Context(), tenantID); err == nil {
		for i := range list {
			if list[i].Key == key {
				override = &list[i]
				break
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"key":      key,
		"default":  def,
		"override": override,
	})
}

func (h *emailTemplateHandler) upsert(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	key := chi.URLParam(r, "key")
	if _, ok := h.reg.Get(key); !ok {
		writeError(w, http.StatusNotFound, "unknown template")
		return
	}

	var body struct {
		Subject string `json:"subject"`
		HTML    string `json:"html"`
		Text    string `json:"text"`
		Enabled *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	tpl := email.Template{Subject: body.Subject, HTML: body.HTML, Text: body.Text}
	if err := email.Validate(tpl); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "template does not compile: "+err.Error())
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if err := h.store.Upsert(r.Context(), tenantID, key, tpl, enabled, &actorID); err != nil {
		writeError(w, http.StatusInternalServerError, "save failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "enabled": enabled})
}

func (h *emailTemplateHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	key := chi.URLParam(r, "key")
	if err := h.store.Delete(r.Context(), tenantID, key); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
