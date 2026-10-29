package mail

import (
	"context"
	"encoding/json"
	"fmt"
)

// Handler is the worker's handler for a job carrying one Mail, sent through m.
func Handler(m Mailer) func(ctx context.Context, payload json.RawMessage) error {
	return func(ctx context.Context, payload json.RawMessage) error {
		var msg Mail
		if err := json.Unmarshal(payload, &msg); err != nil {
			return fmt.Errorf("read the mail job: %w", err)
		}
		return m.Send(ctx, msg)
	}
}
