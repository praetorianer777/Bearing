// Command worker runs the jobs the api leaves on the queue.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/redis/go-redis/v9"

	"github.com/praetorianer777/bearing/backend/internal/auth"
	"github.com/praetorianer777/bearing/backend/internal/config"
	"github.com/praetorianer777/bearing/backend/internal/db"
	"github.com/praetorianer777/bearing/backend/internal/jobs"
	"github.com/praetorianer777/bearing/backend/internal/logging"
	"github.com/praetorianer777/bearing/backend/internal/mail"
	"github.com/praetorianer777/bearing/backend/internal/org"
	"github.com/praetorianer777/bearing/backend/internal/orgcsv"
	"github.com/praetorianer777/bearing/backend/internal/version"
	"github.com/praetorianer777/bearing/backend/internal/webhook"
)

func main() {
	if err := run(); err != nil {
		slog.Error("the worker stopped", "error", err)
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
	log.Info("starting the worker", "build", version.Current(), "env", cfg.Env)

	cluster, err := db.Open(ctx, cfg.DB, log)
	if err != nil {
		return err
	}
	defer cluster.Close()

	valkeyOpts, err := redis.ParseURL(cfg.Valkey.URL)
	if err != nil {
		return fmt.Errorf("BEARING_VALKEY_URL: %w", err)
	}
	valkey := redis.NewClient(valkeyOpts)
	defer valkey.Close()
	if err := valkey.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("reach Valkey: %w", err)
	}

	host, _ := os.Hostname()
	worker := jobs.NewWorker(valkey, cfg.Valkey.JobStream, fmt.Sprintf("%s-%d", host, os.Getpid()), log)
	if cfg.Mail.SMTPAddr != "" {
		worker.Handle(auth.MailJob, mail.Handler(mail.SMTPMailer{Addr: cfg.Mail.SMTPAddr, From: cfg.Mail.From}))
	}
	people := &org.Service{DB: cluster}
	worker.Handle(orgcsv.Job, (&orgcsv.Imports{DB: cluster, Org: people}).Handler())
	hooks, err := webhook.FromConfig(cfg.Outbound, cluster, log)
	if err != nil {
		return err
	}
	hooks.Gaps = func(ctx context.Context, from, to string) ([]webhook.Gap, error) {
		start, err := org.ParseDay(from)
		if err != nil {
			return nil, err
		}
		end, err := org.ParseDay(to)
		if err != nil {
			return nil, err
		}
		gaps, err := people.Gaps(ctx, org.GapQuery{From: start, To: end})
		out := make([]webhook.Gap, len(gaps))
		for i, g := range gaps {
			out[i] = webhook.Gap{Kind: g.Kind, SubjectKind: g.SubjectKind, SubjectID: g.SubjectID, Name: g.Name, UnitID: g.UnitID,
				From: g.From.String(), To: g.To.String(), Size: g.Size}
		}
		return out, err
	}
	go webhook.NewSender(hooks, log).Run(ctx)
	log.Info("reading jobs", "stream", cfg.Valkey.JobStream)
	return worker.Run(ctx)
}
