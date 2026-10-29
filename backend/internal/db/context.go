package db

import (
	"context"
	"sync/atomic"

	"github.com/google/uuid"
)

type (
	pinKey  struct{}
	userKey struct{}
)

// UserVar is the setting the permission policies read through current_actor_id(): whom the transaction acts for.
const UserVar = "app.user_id"

// ViaVar is the setting that names the token or OAuth grant the person acts through, which the database stamps on
// what they write; unset is a session.
const ViaVar = "app.via"

// acting is whom the transactions act for and what they act through, one value so that naming another person
// drops what the last one acted through.
type acting struct {
	user uuid.UUID
	// via is the token or grant the person acts through, uuid.Nil for a session.
	via uuid.UUID
}

// WithUser names the person the transactions made with ctx act for, so row level security holds them to that
// person's permissions.
func WithUser(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userKey{}, acting{user: id})
}

// WithVia names the token or grant the person ctx acts for acts through.
func WithVia(ctx context.Context, id uuid.UUID) context.Context {
	a, ok := ctx.Value(userKey{}).(acting)
	if !ok {
		return ctx
	}
	a.via = id
	return context.WithValue(ctx, userKey{}, a)
}

// ViaFrom is the token or grant WithVia named, if any.
func ViaFrom(ctx context.Context) (uuid.UUID, bool) {
	a, _ := ctx.Value(userKey{}).(acting)
	return a.via, a.via != uuid.Nil
}

// UserFrom is the person WithUser named, if any.
func UserFrom(ctx context.Context) (uuid.UUID, bool) {
	a, ok := ctx.Value(userKey{}).(acting)
	return a.user, ok && a.user != uuid.Nil
}

// pin describes a freshness requirement placed on reads made with a context.
type pin struct {
	// forcePrimary sends every read to the primary regardless of replica state.
	forcePrimary bool
	// requiredLSN is the WAL position a replica must have replayed before it may serve this context's reads.
	requiredLSN LSN
}

// PinPrimary forces every read made with the returned context to the primary, for the rare read that must see the
// absolute latest state.
func PinPrimary(ctx context.Context) context.Context {
	p, _ := ctx.Value(pinKey{}).(pin)
	p.forcePrimary = true
	return context.WithValue(ctx, pinKey{}, p)
}

// PinLSN requires any replica serving this context to have replayed lsn, so a user never reads past their own
// write; otherwise the read goes to the primary.
func PinLSN(ctx context.Context, lsn LSN) context.Context {
	p, _ := ctx.Value(pinKey{}).(pin)
	if lsn > p.requiredLSN {
		p.requiredLSN = lsn
	}
	return context.WithValue(ctx, pinKey{}, p)
}

func pinFrom(ctx context.Context) pin {
	p, _ := ctx.Value(pinKey{}).(pin)
	if t := trackerFrom(ctx); t != nil {
		if seen := LSN(t.seen.Load()); seen > p.requiredLSN {
			p.requiredLSN = seen
		}
	}
	return p
}

type trackerKey struct{}

// readTracker is how far the reads made with one context have seen, so none of them goes back.
type readTracker struct {
	seen      atomic.Uint64
	onPrimary func(LSN)
}

// TrackReads keeps every read made with ctx at least as fresh as the ones before it. onPrimary, when set, hears
// where the primary stood when it answered a read in a replica's place, for the caller's next requests to require.
func TrackReads(ctx context.Context, onPrimary func(LSN)) context.Context {
	return context.WithValue(ctx, trackerKey{}, &readTracker{onPrimary: onPrimary})
}

func trackerFrom(ctx context.Context) *readTracker {
	t, _ := ctx.Value(trackerKey{}).(*readTracker)
	return t
}

// saw records that a read saw the database at lsn, and says whether that is further than any read before it.
func (t *readTracker) saw(lsn LSN) bool {
	for {
		cur := t.seen.Load()
		if uint64(lsn) <= cur {
			return false
		}
		if t.seen.CompareAndSwap(cur, uint64(lsn)) {
			return true
		}
	}
}

// RequiredLSN is the position a replica must have replayed to serve reads made with ctx, for tests and logs.
func RequiredLSN(ctx context.Context) LSN { return pinFrom(ctx).requiredLSN }
