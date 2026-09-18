package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	domain "github.com/imaginacolombia/cms/internal/core/domain"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	userdomain "github.com/imaginacolombia/cms/internal/core/domain/user"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

func MountSections(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &collectionHandler{repos: repos, bus: bus, log: log}
	p := repos.Permission

	r.Route("/collections/{collection}/sections", func(r chi.Router) {
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/", h.listSections)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionCreate)).Post("/", h.createSection)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/reorder", h.reorderSections)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionRead)).Get("/{name}", h.getSection)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionUpdate)).Patch("/{name}", h.updateSection)
		r.With(httpMiddleware.RequirePermission(p, userdomain.VirtualSchema, userdomain.ActionDelete)).Delete("/{name}", h.deleteSection)
	})
}

func (h *collectionHandler) listSections(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	includeFields := r.URL.Query().Get("fields") == "true"

	secs, err := h.repos.Section.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	if includeFields {
		fields, err := h.repos.Field.ListByCollection(r.Context(), tenantID, collName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list fields failed")
			return
		}
		bySec := map[string][]collectiondomain.Field{}
		for _, f := range fields {
			bySec[f.SectionName] = append(bySec[f.SectionName], *f)
		}
		for _, s := range secs {
			s.Fields = bySec[s.Name]
			if s.Fields == nil {
				s.Fields = []collectiondomain.Field{}
			}
		}
	}
	writeJSON(w, http.StatusOK, secs)
}

func (h *collectionHandler) createSection(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	coll, err := h.repos.Collection.GetByName(r.Context(), tenantID, collName)
	if err != nil || coll == nil {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	}

	var body struct {
		Name         string                         `json:"name"`
		DisplayName  string                         `json:"display_name"`
		Note         *string                        `json:"note"`
		Sort         int                            `json:"sort"`
		Visibility   *collectiondomain.Condition    `json:"visibility"`
		Translations []collectiondomain.Translation `json:"translations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if body.DisplayName == "" {
		body.DisplayName = body.Name
	}

	known := knownFieldNames(coll)
	if err := body.Visibility.Validate(known); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	sec := &collectiondomain.Section{
		Base:           domain.NewBase(tenantID, &actorID),
		CollectionName: collName,
		Name:           body.Name,
		DisplayName:    body.DisplayName,
		Note:           body.Note,
		Sort:           body.Sort,
		Visibility:     body.Visibility,
		Translations:   body.Translations,
	}
	if sec.Translations == nil {
		sec.Translations = []collectiondomain.Translation{}
	}

	if err := h.repos.Section.Create(r.Context(), tenantID, sec); err != nil {
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}
	writeJSON(w, http.StatusCreated, sec)
}

func (h *collectionHandler) getSection(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	if sec == nil {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}
	fields, err := h.repos.Field.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list fields failed")
		return
	}
	for _, f := range fields {
		if f.SectionID == sec.ID {
			sec.Fields = append(sec.Fields, *f)
		}
	}
	if sec.Fields == nil {
		sec.Fields = []collectiondomain.Field{}
	}
	writeJSON(w, http.StatusOK, sec)
}

func (h *collectionHandler) updateSection(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, name)
	if err != nil || sec == nil {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}

	var body struct {
		DisplayName  *string                         `json:"display_name"`
		Note         *string                         `json:"note"`
		Sort         *int                            `json:"sort"`
		// RawMessage so we can distinguish omitted vs explicit null (clear).
		Visibility   json.RawMessage                 `json:"visibility"`
		Translations *[]collectiondomain.Translation `json:"translations"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	if body.DisplayName != nil {
		sec.DisplayName = *body.DisplayName
	}
	if body.Note != nil {
		sec.Note = body.Note
	}
	if body.Sort != nil {
		sec.Sort = *body.Sort
	}
	if body.Visibility != nil {
		if string(body.Visibility) == "null" {
			sec.Visibility = nil
		} else {
			var c collectiondomain.Condition
			if err := json.Unmarshal(body.Visibility, &c); err != nil {
				writeError(w, http.StatusBadRequest, "invalid visibility")
				return
			}
			coll, _ := h.repos.Collection.GetByName(r.Context(), tenantID, collName)
			known := knownFieldNames(coll)
			if err := c.Validate(known); err != nil {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return
			}
			sec.Visibility = &c
		}
	}
	if body.Translations != nil {
		sec.Translations = *body.Translations
	}
	sec.Touch(&actorID)

	if err := h.repos.Section.Update(r.Context(), tenantID, sec); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, sec)
}

func (h *collectionHandler) deleteSection(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")
	name := chi.URLParam(r, "name")

	count, err := h.repos.Section.CountByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "count failed")
		return
	}
	if count <= 1 {
		writeError(w, http.StatusConflict, "at least one section must remain")
		return
	}

	sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, name)
	if err != nil || sec == nil {
		writeError(w, http.StatusNotFound, "section not found")
		return
	}

	fields, err := h.repos.Field.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list fields failed")
		return
	}
	hasFields := false
	for _, f := range fields {
		if f.SectionID == sec.ID {
			hasFields = true
			break
		}
	}

	reassignTo := r.URL.Query().Get("reassign_to")
	if hasFields {
		if reassignTo == "" {
			var body struct {
				ReassignTo string `json:"reassign_to"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			reassignTo = body.ReassignTo
		}
		if reassignTo == "" {
			writeError(w, http.StatusConflict, "section has fields; provide reassign_to")
			return
		}
		target, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, reassignTo)
		if err != nil || target == nil {
			writeError(w, http.StatusUnprocessableEntity, "reassign_to section not found")
			return
		}
		if target.ID == sec.ID {
			writeError(w, http.StatusUnprocessableEntity, "reassign_to must be a different section")
			return
		}
		if err := h.repos.Section.ReassignFields(r.Context(), tenantID, sec.ID, target.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "reassign failed")
			return
		}
	}

	if err := h.repos.Section.Delete(r.Context(), tenantID, collName, name); err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *collectionHandler) reorderSections(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	actorID, _ := httpMiddleware.UserIDFromCtx(r.Context())
	collName := chi.URLParam(r, "collection")

	var body struct {
		Order []string `json:"order"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(body.Order) == 0 {
		writeError(w, http.StatusBadRequest, "order must be a non-empty array of section names")
		return
	}

	for i, name := range body.Order {
		sec, err := h.repos.Section.GetByName(r.Context(), tenantID, collName, name)
		if err != nil || sec == nil {
			continue
		}
		sec.Sort = i
		sec.Touch(&actorID)
		if err := h.repos.Section.Update(r.Context(), tenantID, sec); err != nil {
			h.log.Warnw("reorder: update sort failed", "section", name, "err", err)
		}
	}

	secs, err := h.repos.Section.ListByCollection(r.Context(), tenantID, collName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	writeJSON(w, http.StatusOK, secs)
}

func knownFieldNames(coll *collectiondomain.Collection) map[string]bool {
	if coll == nil {
		return nil
	}
	known := map[string]bool{}
	for _, f := range coll.AllFields() {
		known[f.Name] = true
	}
	return known
}