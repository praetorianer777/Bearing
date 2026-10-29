// Command api serves Bearing's HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/praetorianer777/bearing/backend/internal/app"
	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/logging"
	"github.com/praetorianer777/bearing/backend/internal/version"
)

// readHeaderTimeout bounds how long a client may take to send its headers, so slow clients cannot hold
// connections open.
const readHeaderTimeout = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("the api stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.RequireValkey(); err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting the api", "build", version.Current(), "env", cfg.Env)

	cluster, err := db.Open(ctx, cfg.DB, log)
	if err != nil {
		return err
	}
	defer cluster.Close()
	if err := cluster.RefuseSuperuser(ctx); err != nil {
		return err
	}

	valkeyOpts, err := redis.ParseURL(cfg.Valkey.URL)
	if err != nil {
		return fmt.Errorf("BEARING_VALKEY_URL: %w", err)
	}
	valkey := redis.NewClient(valkeyOpts)
	defer valkey.Close()
	if err := valkey.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("reach Valkey: %w", err)
	}

	srv, err := app.NewServer(cfg, cluster, valkey, log)
	if err != nil {
		return err
	}
	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: readHeaderTimeout,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", cfg.HTTP.Addr)
		errc <- httpServer.ListenAndServe()
	}()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
