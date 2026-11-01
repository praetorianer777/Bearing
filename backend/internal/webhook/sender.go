package webhook

import (
	"context"
	"log/slog"
	"time"
)

// Sender announces changes and posts due deliveries until its context ends, and prunes the log. Any number may
// run: each change and each delivery is claimed before it is handled.
type Sender struct {
	svc      *Service
	log      *slog.Logger
	interval time.Duration
	pruned   time.Time
}

// NewSender runs svc every SendInterval.
func NewSender(svc *Service, log *slog.Logger) *Sender {
	return &Sender{svc: svc, log: log, interval: SendInterval}
}

// Run sends until ctx ends.
func (s *Sender) Run(ctx context.Context) {
	s.log.Info("webhook sender started", "interval", s.interval)
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		changes, err := s.svc.FanOut(ctx)
		if err != nil && ctx.Err() == nil {
			s.log.Warn("changes could not be announced to webhooks", "error", err)
		}
		turned, err := s.svc.Sweep(ctx)
		if err != nil && ctx.Err() == nil {
			s.log.Warn("the holdings starting and ending today could not be announced", "error", err)
		}
		if _, err := s.svc.CheckGaps(ctx, changes+turned > 0); err != nil && ctx.Err() == nil {
			s.log.Warn("the gaps could not be compared", "error", err)
		}
		if _, err := s.svc.SendDue(ctx); err != nil && ctx.Err() == nil {
			s.log.Warn("due webhook deliveries could not be claimed", "error", err)
		}
		if time.Since(s.pruned) > pruneEvery {
			s.pruned = time.Now()
			if _, err := s.svc.Prune(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("old webhook deliveries could not be pruned", "error", err)
			}
		}
		select {
		case <-ctx.Done():
			s.log.Info("webhook sender stopped")
			return
		case <-ticker.C:
		}
	}
}
