package rest

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/imaginacolombia/cms/internal/infrastructure/database"
)

func MountHealth(r chi.Router, db *pgxpool.Pool) {
	r.Get("/health", healthHandler(db))
}

func healthHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dbStatus := "ok"
		if err := database.HealthCheck(r.Context(), db); err != nil {
			dbStatus = "error: " + err.Error()
		}

		status := http.StatusOK
		if dbStatus != "ok" {
			status = http.StatusServiceUnavailable
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"status": map[string]string{
				"api":      "ok",
				"database": dbStatus,
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
