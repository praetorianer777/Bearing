package db

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestNamingAnotherPersonDropsWhatTheLastActedThrough(t *testing.T) {
	alice, bob, token := uuid.New(), uuid.New(), uuid.New()
	ctx := WithVia(WithUser(context.Background(), alice), token)
	if via, ok := ViaFrom(ctx); !ok || via != token {
		t.Fatalf("via = %v %v, want the token", via, ok)
	}
	ctx = WithUser(ctx, bob)
	if _, ok := ViaFrom(ctx); ok {
		t.Error("bob acts through alice's token")
	}
	if user, _ := UserFrom(ctx); user != bob {
		t.Errorf("acting for %v, want bob", user)
	}
}

func TestViaNeedsAPerson(t *testing.T) {
	if _, ok := ViaFrom(WithVia(context.Background(), uuid.New())); ok {
		t.Error("a token was named for nobody")
	}
}

func TestNobodyIsNamedUntilSomebodyIs(t *testing.T) {
	if _, ok := UserFrom(context.Background()); ok {
		t.Error("a bare context names somebody")
	}
	if _, ok := UserFrom(WithUser(context.Background(), uuid.Nil)); ok {
		t.Error("the nil id counts as a person")
	}
}

func TestPinsOnlyEverRise(t *testing.T) {
	ctx := PinLSN(PinLSN(context.Background(), 200), 100)
	if got := pinFrom(ctx).requiredLSN; got != 200 {
		t.Errorf("the pin fell back to %d, want 200", got)
	}
	if !pinFrom(PinPrimary(ctx)).forcePrimary || pinFrom(PinPrimary(ctx)).requiredLSN != 200 {
		t.Error("pinning to the primary lost the position")
	}
}

// What one read saw holds every later read made with the context to it, and only a step forward is news.
func TestAReadNeverGoesBackOnWhatAnEarlierOneSaw(t *testing.T) {
	ctx := TrackReads(PinLSN(context.Background(), 100), nil)
	tracker := trackerFrom(ctx)
	if got := pinFrom(ctx).requiredLSN; got != 100 {
		t.Fatalf("before any read the pin is %d, want the caller's own 100", got)
	}
	if !tracker.saw(300) {
		t.Error("a read seeing further than any before was not news")
	}
	if tracker.saw(200) || tracker.saw(300) {
		t.Error("a read seeing less, or as much, was taken for news")
	}
	if got := pinFrom(ctx).requiredLSN; got != 300 {
		t.Errorf("after a read saw 300 the pin is %d, want 300", got)
	}
	if trackerFrom(context.Background()) != nil {
		t.Error("an untracked context has a tracker")
	}
}
