package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httpadapter "github.com/imaginacolombia/cms/internal/adapters/http"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

type Server struct {
	cfg    *config.Config
	log    *logger.Logger
	db     *pgxpool.Pool
	bus    events.Bus
	httpSrv *http.Server
}

func New(cfg *config.Config, log *logger.Logger, db *pgxpool.Pool, bus events.Bus) *Server {
	return &Server{cfg: cfg, log: log, db: db, bus: bus}
}

func (s *Server) Start(ctx context.Context) error {
	router := httpadapter.NewRouter(ctx, s.cfg, s.log, s.db, s.bus)

	s.httpSrv = &http.Server{
		Addr:         fmt.Sprintf("%s:%s", s.cfg.Server.Host, s.cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
	}

	s.log.Infow("server starting",
		"addr", s.httpSrv.Addr,
		"env", s.cfg.App.Environment,
	)

	go func() {
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Errorw("server error", "err", err)
		}
	}()

	return nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpSrv.Shutdown(ctx)
}
