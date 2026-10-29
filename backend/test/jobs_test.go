//go:build integration

package test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/praetorianer777/bearing/backend/internal/jobs"
)

// stream is a stream of the test's own, removed when it ends.
func stream(t *testing.T) string {
	t.Helper()
	name := "bearing:test:" + t.Name() + ":" + time.Now().Format("150405.000000")
	client := valkey(t)
	t.Cleanup(func() { client.Del(context.Background(), name, jobs.DeadStream(name)) })
	return name
}

func TestAJobRunsOnceAndIsGone(t *testing.T) {
	ctx := context.Background()
	client, name := valkey(t), stream(t)
	if _, err := jobs.NewQueue(client, name).Enqueue(ctx, "greet", map[string]string{"to": "alice"}); err != nil {
		t.Fatal(err)
	}
	var got []string
	w := jobs.NewWorker(client, name, "test", quietLog())
	w.Handle("greet", func(_ context.Context, payload json.RawMessage) error {
		var p struct{ To string }
		if err := json.Unmarshal(payload, &p); err != nil {
			return err
		}
		got = append(got, p.To)
		return nil
	})
	for range 2 {
		if err := w.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if len(got) != 1 || got[0] != "alice" {
		t.Errorf("the handler saw %q, want alice once", got)
	}
	if n := client.XLen(ctx, name).Val(); n != 0 {
		t.Errorf("%d jobs left on the stream", n)
	}
}

func TestAFailingJobIsTriedAgainThenGivenUp(t *testing.T) {
	ctx := context.Background()
	client, name := valkey(t), stream(t)
	if _, err := jobs.NewQueue(client, name).Enqueue(ctx, "flaky", nil); err != nil {
		t.Fatal(err)
	}
	attempts := 0
	w := jobs.NewWorker(client, name, "test", quietLog())
	w.MaxAttempts = 3
	w.RetryAfter = 10 * time.Millisecond
	w.Handle("flaky", func(context.Context, json.RawMessage) error {
		attempts++
		return errors.New("not today")
	})
	for range 6 {
		if err := w.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if attempts != 3 {
		t.Errorf("tried %d times, want 3", attempts)
	}
	dead := client.XRange(ctx, jobs.DeadStream(name), "-", "+").Val()
	if len(dead) != 1 || dead[0].Values["reason"] != "not today" || dead[0].Values["kind"] != "flaky" {
		t.Errorf("the dead stream holds %v", dead)
	}
}

func TestAJobNobodyHandlesIsGivenUpAtOnce(t *testing.T) {
	ctx := context.Background()
	client, name := valkey(t), stream(t)
	if _, err := jobs.NewQueue(client, name).Enqueue(ctx, "unknown", nil); err != nil {
		t.Fatal(err)
	}
	if err := jobs.NewWorker(client, name, "test", quietLog()).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if n := client.XLen(ctx, jobs.DeadStream(name)).Val(); n != 1 {
		t.Errorf("%d jobs given up, want 1", n)
	}
}

func TestAJobNeedsAKind(t *testing.T) {
	if _, err := jobs.NewQueue(valkey(t), stream(t)).Enqueue(context.Background(), "", nil); err == nil {
		t.Error("a job without a kind was added")
	}
}
