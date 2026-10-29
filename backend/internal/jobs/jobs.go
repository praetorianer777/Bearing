// Package jobs is the worker's queue: the api adds a job to a Valkey stream, and the worker reads it in a consumer
// group, runs the handler registered for its kind and acknowledges it. A job whose handler fails is claimed again
// after a pause, and one that failed MaxAttempts times, or whose kind nobody handles, moves to the dead stream.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Defaults the worker runs with unless the caller sets others.
const (
	DefaultGroup       = "workers"
	DefaultMaxAttempts = 5
	DefaultRetryAfter  = time.Minute
	DefaultBlock       = 5 * time.Second
)

// Handler does one job's work with its payload. Returning an error has the job tried again later.
type Handler func(ctx context.Context, payload json.RawMessage) error

// Queue adds jobs to the stream.
type Queue struct {
	client *redis.Client
	stream string
}

// NewQueue adds to stream through client.
func NewQueue(client *redis.Client, stream string) *Queue {
	return &Queue{client: client, stream: stream}
}

// Enqueue adds a job of kind with payload, marshalled as JSON, and returns its id.
func (q *Queue) Enqueue(ctx context.Context, kind string, payload any) (string, error) {
	if kind == "" {
		return "", errors.New("a job needs a kind")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal the %s job: %w", kind, err)
	}
	id, err := q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: q.stream,
		Values: map[string]any{"kind": kind, "payload": string(body)},
	}).Result()
	if err != nil {
		return "", fmt.Errorf("add the %s job: %w", kind, err)
	}
	return id, nil
}

// DeadStream is where jobs that will not succeed are kept for a person to look at.
func DeadStream(stream string) string { return stream + ":dead" }

// Worker reads the stream in a consumer group and runs the registered handlers.
type Worker struct {
	client   *redis.Client
	stream   string
	group    string
	consumer string
	handlers map[string]Handler
	log      *slog.Logger

	MaxAttempts int64
	RetryAfter  time.Duration
	Block       time.Duration
}

// NewWorker reads stream as consumer, which must be unique among the workers running at once.
func NewWorker(client *redis.Client, stream, consumer string, log *slog.Logger) *Worker {
	return &Worker{
		client:      client,
		stream:      stream,
		group:       DefaultGroup,
		consumer:    consumer,
		handlers:    map[string]Handler{},
		log:         log,
		MaxAttempts: DefaultMaxAttempts,
		RetryAfter:  DefaultRetryAfter,
		Block:       DefaultBlock,
	}
}

// Handle registers the handler for kind; a kind registered twice is a programming error.
func (w *Worker) Handle(kind string, h Handler) {
	if _, dup := w.handlers[kind]; dup {
		panic("jobs: two handlers for " + kind)
	}
	w.handlers[kind] = h
}

// Run works until ctx ends.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.ensureGroup(ctx); err != nil {
		return err
	}
	for ctx.Err() == nil {
		if err := w.reclaim(ctx); err != nil && ctx.Err() == nil {
			w.log.Warn("reclaiming jobs failed", "error", err)
		}
		if err := w.readNew(ctx); err != nil && ctx.Err() == nil {
			w.log.Warn("reading jobs failed", "error", err)
			sleep(ctx, time.Second)
		}
	}
	return nil
}

// RunOnce reads what is waiting and what is due again, without blocking, for tests and one-shot runs.
func (w *Worker) RunOnce(ctx context.Context) error {
	if err := w.ensureGroup(ctx); err != nil {
		return err
	}
	if err := w.reclaim(ctx); err != nil {
		return err
	}
	block := w.Block
	w.Block = -1
	defer func() { w.Block = block }()
	return w.readNew(ctx)
}

func (w *Worker) ensureGroup(ctx context.Context) error {
	err := w.client.XGroupCreateMkStream(ctx, w.stream, w.group, "0").Err()
	if err != nil && !strings.HasPrefix(err.Error(), "BUSYGROUP") {
		return fmt.Errorf("make the consumer group %s on %s: %w", w.group, w.stream, err)
	}
	return nil
}

func (w *Worker) readNew(ctx context.Context) error {
	streams, err := w.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    w.group,
		Consumer: w.consumer,
		Streams:  []string{w.stream, ">"},
		Count:    10,
		Block:    w.Block,
	}).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, s := range streams {
		for _, msg := range s.Messages {
			w.process(ctx, msg, 1)
		}
	}
	return nil
}

// reclaim takes over the jobs that failed, or whose worker died, RetryAfter ago.
func (w *Worker) reclaim(ctx context.Context) error {
	pending, err := w.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: w.stream,
		Group:  w.group,
		Idle:   w.RetryAfter,
		Start:  "-",
		End:    "+",
		Count:  10,
	}).Result()
	if err != nil {
		return err
	}
	for _, p := range pending {
		msgs, err := w.client.XClaim(ctx, &redis.XClaimArgs{
			Stream:   w.stream,
			Group:    w.group,
			Consumer: w.consumer,
			MinIdle:  w.RetryAfter,
			Messages: []string{p.ID},
		}).Result()
		if err != nil {
			return err
		}
		for _, msg := range msgs {
			w.process(ctx, msg, p.RetryCount+1)
		}
	}
	return nil
}

// process runs one job; attempt counts this delivery.
func (w *Worker) process(ctx context.Context, msg redis.XMessage, attempt int64) {
	kind, _ := msg.Values["kind"].(string)
	payload, _ := msg.Values["payload"].(string)
	h, ok := w.handlers[kind]
	if !ok {
		w.bury(ctx, msg, kind, "no handler for this kind", attempt)
		return
	}
	err := h(ctx, json.RawMessage(payload))
	if err == nil {
		w.done(ctx, msg.ID)
		return
	}
	if attempt >= w.MaxAttempts {
		w.bury(ctx, msg, kind, err.Error(), attempt)
		return
	}
	w.log.Warn("job failed; it will be tried again", "kind", kind, "id", msg.ID, "attempt", attempt, "error", err)
}

func (w *Worker) done(ctx context.Context, id string) {
	pipe := w.client.TxPipeline()
	pipe.XAck(ctx, w.stream, w.group, id)
	pipe.XDel(ctx, w.stream, id)
	if _, err := pipe.Exec(ctx); err != nil {
		w.log.Warn("acknowledging a job failed; it may run again", "id", id, "error", err)
	}
}

func (w *Worker) bury(ctx context.Context, msg redis.XMessage, kind, reason string, attempt int64) {
	w.log.Error("job given up", "kind", kind, "id", msg.ID, "attempts", attempt, "reason", reason)
	values := map[string]any{"id": msg.ID, "reason": reason, "attempts": strconv.FormatInt(attempt, 10)}
	for k, v := range msg.Values {
		values[k] = v
	}
	if err := w.client.XAdd(ctx, &redis.XAddArgs{Stream: DeadStream(w.stream), Values: values}).Err(); err != nil {
		w.log.Error("keeping a given up job failed; it stays pending", "id", msg.ID, "error", err)
		return
	}
	w.done(ctx, msg.ID)
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
