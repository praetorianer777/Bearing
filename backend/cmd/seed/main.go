// Command seed sets the installation up with its name and first administrator from BEARING_NAME and
// BEARING_BOOTSTRAP_ADMIN_*, when it is not set up yet. Run again, it changes nothing.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	switch {
	case errors.Is(err, auth.ErrAlreadySetUp):
		log.Info("the installation is set up already")
	case err != nil:
		return err
	default:
		log.Info("installation set up", "name", cfg.Bootstrap.Name, "administrator", cfg.Bootstrap.AdminEmail)
	}
	members, err := parseMembers(cfg.Bootstrap.Members)
	if err != nil {
		return fmt.Errorf("BEARING_BOOTSTRAP_MEMBERS: %w", err)
	}
	if len(members) == 0 {
		return nil
	}
	if err := accounts.Prepare(ctx, members); err != nil {
		return err
	}
	log.Info("members let in ahead of their first single sign-on", "members", len(members))
	return nil
}

// parseMembers reads "email=role" pairs, separated by commas.
func parseMembers(spec string) (map[string]auth.Role, error) {
	out := map[string]auth.Role{}
	for pair := range strings.SplitSeq(spec, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		email, role, ok := strings.Cut(pair, "=")
		if !ok || !auth.Role(role).Valid() {
			return nil, fmt.Errorf("%q is not email=role with a role of administrator, editor, member or reader", pair)
		}
		out[strings.TrimSpace(email)] = auth.Role(role)
	}
	return out, nil
}
