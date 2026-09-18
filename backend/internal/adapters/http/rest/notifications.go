package rest

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/notifications"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountNotifications registers inbox endpoints for the authenticated user.
// Creating notifications from other features uses notifications.Service (injected
// into handlers that need it), not these routes.
func MountNotifications(r chi.Router, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &notificationHandler{repos: repos, bus: bus, log: log}
	r.Route("/notifications", func(r chi.Router) {
		r.Get("/", h.list)
		r.Get("/unread-count", h.unreadCount)
		r.Post("/read-all", h.markAllRead)
		r.Post("/{id}/read", h.markRead)
	})
}

type notificationHandler struct {
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

func (h *notificationHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	page, limit := parsePage(r)
	filter := outbound.NotificationFilter{
		Page:       page,
		Limit:      limit,
		UnreadOnly: r.URL.Query().Get("unread") == "true" || r.URL.Query().Get("unread") == "1",
	}
	items, total, err := h.repos.Notification.ListForUser(r.Context(), tenantID, userID, filter)
	if err != nil {
		h.log.Errorw("notification list failed", "err", err)
		writeError(w, http.StatusInternalServerError, "list failed")
		return
	}
	paginated(w, items, total, page, limit)
}

func (h *notificationHandler) unreadCount(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	n, err := h.repos.Notification.CountUnread(r.Context(), tenantID, userID)
	if err != nil {
		h.log.Errorw("notification unread count failed", "err", err)
		writeError(w, http.StatusInternalServerError, "count failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"count": n})
}

func (h *notificationHandler) markRead(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.repos.Notification.MarkRead(r.Context(), tenantID, userID, id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		h.log.Errorw("notification mark read failed", "err", err)
		writeError(w, http.StatusInternalServerError, "mark read failed")
		return
	}
	_ = h.bus.Publish(r.Context(), events.Event{
		Type:     events.EventNotificationRead,
		TenantID: tenantID,
		UserID:   &userID,
		Payload:  map[string]any{"id": id.String()},
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *notificationHandler) markAllRead(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.repos.Notification.MarkAllRead(r.Context(), tenantID, userID); err != nil {
		h.log.Errorw("notification mark all read failed", "err", err)
		writeError(w, http.StatusInternalServerError, "mark all read failed")
		return
	}
	_ = h.bus.Publish(r.Context(), events.Event{
		Type:     events.EventNotificationRead,
		TenantID: tenantID,
		UserID:   &userID,
		Payload:  map[string]any{"all": true},
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Notify is a thin rest-package helper for other handlers in this package that
// already hold *postgresrepo.Repos + events.Bus. Prefer injecting
// *notifications.Service when wiring new features.
func Notify(r *http.Request, repos *postgresrepo.Repos, bus events.Bus, recipientID uuid.UUID, in notifications.Input) error {
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	svc := notifications.NewService(repos.Notification, bus)
	_, err := svc.ToUser(r.Context(), tenantID, recipientID, in)
	return err
}
