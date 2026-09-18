package middleware

import (
	"encoding/json"
	"net/http"
	"runtime/debug"

	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// Recoverer catches panics from any downstream middleware or handler and always
// responds with a JSON 500 body, so API clients never receive an empty page on a
// crash. The panic value and stack are logged server-side (via zap); the client
// only sees a generic message — internal details are never leaked in the response.
//
// This replaces chi's middleware.Recoverer, which writes an empty/plain body.
func Recoverer(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				// http.ErrAbortHandler is a sentinel used to intentionally abort
				// a response; propagate it rather than swallowing it.
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				log.Errorw("panic recovered",
					"err", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
