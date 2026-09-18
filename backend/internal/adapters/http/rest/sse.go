package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountSSE registers GET /events on a route group that does NOT use the
// standard Authenticate middleware, because the browser's EventSource API
// cannot set custom headers. Auth is handled inline via the ?token= query param.
func MountSSE(r chi.Router, cfg *config.Config, bus events.Bus, log *logger.Logger) {
	r.Get("/events", sseHandler(cfg.Auth.JWTSecret, bus, log))
}

// sseHandler streams typed events to authenticated browser clients.
//
// Why SSE (not WebSocket):
//   - All realtime traffic is server→client (data changes, notifications)
//   - EventSource reconnects automatically — zero client reconnect code
//   - Works through every HTTP proxy and load balancer unchanged
//   - HTTP/2 multiplexes streams on one TCP connection for free
//   - No upgrade handshake; simpler server implementation
func sseHandler(jwtSecret string, bus events.Bus, log *logger.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ── Auth via ?token= (EventSource cannot set headers) ─────────────────
		claims, err := jwtFromQuery(r, jwtSecret)
		if err != nil {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}

		userID, _ := uuid.Parse(claims.Subject)
		tenantID, _ := uuid.Parse(claims.TenantID)

		// ── SSE response headers ──────────────────────────────────────────────
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no") // prevents nginx from buffering the stream

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Confirm connection immediately so the client knows it's live.
		sendEvent(w, flusher, "connected", map[string]string{
			"user_id":   userID.String(),
			"tenant_id": tenantID.String(),
		})

		// ── Subscribe to all relevant event types for this tenant ─────────────
		ch := make(chan events.Event, 64)

		subscribedTypes := []events.EventType{
			events.EventAfterCreate, events.EventAfterUpdate, events.EventAfterDelete,
			events.EventCollectionCreated, events.EventCollectionUpdated, events.EventCollectionDeleted,
			events.EventFieldCreated, events.EventFieldUpdated, events.EventFieldDeleted,
			events.EventUserCreated,
			events.EventNotificationCreated,
			events.EventNotificationRead,
		}

		subs := make([]events.Subscription, 0, len(subscribedTypes))
		for _, et := range subscribedTypes {
			et := et
			sub := bus.Subscribe(et, func(_ context.Context, ev events.Event) error {
				if ev.TenantID != tenantID {
					return nil // only forward events that belong to this tenant
				}
				// Inbox events are private — only the recipient may see them.
				if et == events.EventNotificationCreated || et == events.EventNotificationRead {
					if ev.UserID == nil || *ev.UserID != userID {
						return nil
					}
				}
				select {
				case ch <- ev:
				default: // drop if the buffer is full (slow client)
				}
				return nil
			})
			subs = append(subs, sub)
		}
		defer func() {
			for _, sub := range subs {
				bus.Unsubscribe(sub)
			}
		}()

		// Keep-alive comment every 25 s prevents upstream proxy timeouts.
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		log.Infow("sse connected", "user_id", userID, "tenant_id", tenantID, "remote", r.RemoteAddr)

		for {
			select {
			case <-r.Context().Done():
				log.Infow("sse disconnected", "user_id", userID)
				return

			case ev := <-ch:
				sendEvent(w, flusher, string(ev.Type), ev)

			case <-ticker.C:
				// SSE comment — browsers ignore it; proxies see activity and stay alive.
				fmt.Fprintf(w, ": ping\n\n") //nolint:errcheck
				flusher.Flush()
			}
		}
	}
}

// sendEvent serialises payload to JSON and writes one SSE message.
func sendEvent(w http.ResponseWriter, f http.Flusher, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data) //nolint:errcheck
	f.Flush()
}

// jwtFromQuery validates the SSE JWT. EventSource cannot set headers, so the
// token comes from the ?token= query param or, for the same-origin cookie-auth
// SPA, the httpOnly access_token cookie the browser sends automatically.
func jwtFromQuery(r *http.Request, secret string) (*middleware.Claims, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		if c, err := r.Cookie("access_token"); err == nil {
			token = c.Value
		}
	}
	if token == "" {
		return nil, jwt.ErrTokenMalformed
	}
	claims := &middleware.Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}
