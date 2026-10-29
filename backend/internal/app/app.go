// Package app assembles the api from its configuration, so the command and the integration suite build the same
// server.
package app

import (
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/freshness"
	"github.com/praetorianer777/bearing/backend/internal/httpapi"
	"github.com/praetorianer777/bearing/backend/internal/jobs"
)

// NewServer is the api's HTTP server over cluster and valkey, as cfg sets it up.
func NewServer(cfg config.Config, cluster *db.Cluster, valkey *redis.Client, log *slog.Logger) *httpapi.Server {
	accounts := &auth.Service{
		DB:            cluster,
		Sessions:      auth.NewSessions(valkey, cfg.Auth.SessionTTL),
		AppURL:        cfg.Auth.AppURL,
		Params:        auth.DefaultPasswordParams(),
		InvitationTTL: cfg.Auth.InvitationTTL,
		ResetTTL:      cfg.Auth.ResetTTL,
	}
	if cfg.Mail.SMTPAddr != "" {
		accounts.Mail = jobs.NewQueue(valkey, cfg.Valkey.JobStream)
	}
	return &httpapi.Server{
		DB:             cluster,
		Fresh:          freshness.NewValkeyTracker(valkey, cfg.DB.ReadYourWritesTTL, freshness.Prefix),
		Log:            log,
		Accounts:       accounts,
		Auth:           accounts,
		SessionCookie:  cfg.Auth.SessionCookie,
		SessionTTL:     cfg.Auth.SessionTTL,
		MailOn:         cfg.Mail.SMTPAddr != "",
		Secure:         cfg.HTTP.SecureCookies,
		RequestTimeout: cfg.HTTP.RequestTimeout,
	}
}
