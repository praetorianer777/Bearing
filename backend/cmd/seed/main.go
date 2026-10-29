// Command seed sets the installation up with its name and first administrator from BEARING_NAME and
// BEARING_BOOTSTRAP_ADMIN_*, when it is not set up yet. Run again, it changes nothing.
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/logging"
)

func main() {
	if err := run(); err != nil {
		slog.Error("seeding failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New(cfg.LogLevel)
	if cfg.Bootstrap.AdminEmail == "" {
		log.Info("no first administrator named; whoever opens the web client first sets the installation up")
		return nil
	}
	cluster, err := db.Open(ctx, cfg.DB, log)
	if err != nil {
		return err
	}
	defer cluster.Close()
	accounts := &auth.Service{DB: cluster, Params: auth.DefaultPasswordParams()}
	_, err = accounts.SetUp(ctx, cfg.Bootstrap.Name, cfg.Bootstrap.AdminName, cfg.Bootstrap.AdminEmail, cfg.Bootstrap.AdminPassword)
	if errors.Is(err, auth.ErrAlreadySetUp) {
		log.Info("the installation is set up already; nothing to do")
		return nil
	}
	if err != nil {
		return err
	}
	log.Info("installation set up", "name", cfg.Bootstrap.Name, "administrator", cfg.Bootstrap.AdminEmail)
	return nil
}
