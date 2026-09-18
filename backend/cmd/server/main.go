package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"net/http"

	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/infrastructure/bootstrap"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/database"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
	"github.com/imaginacolombia/cms/internal/infrastructure/server"
)

func main() {
	// health-check flag for Docker
	if len(os.Args) > 1 && os.Args[1] == "--health" {
		os.Exit(healthCheck())
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := config.Load()

	log := logger.New(cfg.App.Environment)
	defer log.Sync() //nolint:errcheck

	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		log.Fatal(fmt.Sprintf("database connection failed: %v", err))
	}
	defer db.Close()

	if err := database.Migrate(cfg.Database); err != nil {
		log.Fatal(fmt.Sprintf("migrations failed: %v", err))
	}

	if err := bootstrap.Run(ctx, cfg, db, log); err != nil {
		log.Fatal(fmt.Sprintf("bootstrap failed: %v", err))
	}

	bus := events.NewInMemoryBus()
	bus.SetErrorHandler(func(event events.Event, err error) {
		log.Errorw("event handler failed", "type", event.Type, "collection", event.Collection, "err", err)
	})

	srv := server.New(cfg, log, db, bus)

	if err := srv.Start(ctx); err != nil {
		log.Fatal(fmt.Sprintf("server error: %v", err))
	}

	<-ctx.Done()
	log.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error(fmt.Sprintf("graceful shutdown failed: %v", err))
	}

	log.Info("server stopped")
}

func healthCheck() int {
	// minimal health probe used by Docker HEALTHCHECK
	resp, err := http.Get("http://localhost:" + config.Load().Server.Port + "/api/health")
	if err != nil || resp.StatusCode != 200 {
		return 1
	}
	return 0
}
