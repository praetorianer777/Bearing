package org

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestADayIsWrittenAsTheCalendarHasIt(t *testing.T) {
	d, err := ParseDay("2026-03-01")
	if err != nil || d.String() != "2026-03-01" {
		t.Fatalf("%v %v", d, err)
	}
	if _, err := ParseDay("1.3.2026"); err == nil {
		t.Error("a day in another shape was read")
	}
	late := time.Date(2026, 3, 1, 23, 30, 0, 0, time.FixedZone("CET", 3600))
	if DayOf(late) != d {
		t.Errorf("half past eleven at night on the first is %s", DayOf(late))
	}
	raw, _ := json.Marshal(struct{ On Day }{d})
	if string(raw) != `{"On":"2026-03-01"}` {
		t.Errorf("marshalled %s", raw)
	}
	var back struct{ On Day }
	if err := json.Unmarshal(raw, &back); err != nil || back.On != d {
		t.Errorf("unmarshalled %v %v", back.On, err)
	}
	for _, bad := range []string{`{"On":3}`, `{"On":"tomorrow"}`} {
		if err := json.Unmarshal([]byte(bad), &back); err == nil {
			t.Errorf("%s was read", bad)
		}
	}
	if !d.Before(DayOf(d.Time().AddDate(0, 0, 1))) || d.IsZero() || !(Day{}).IsZero() {
		t.Error("days do not order")
	}
	if dayPtr(nil) != nil || *dayPtr(&late) != d {
		t.Error("dayPtr")
	}
}

func ptr[T any](v T) *T { return &v }

func TestInputIsCheckedBeforeAnythingIsWritten(t *testing.T) {
	s := &Service{}
	for name, in := range map[string]UnitTypeInput{
		"no name":     {},
		"empty name":  {Name: ptr("  ")},
		"long name":   {Name: ptr(strings.Repeat("x", 101))},
		"an odd icon": {Name: ptr("Team"), Icon: ptr("Team!")},
		"a colour":    {Name: ptr("Team"), Colour: ptr("blue")},
	} {
		var invalid Invalid
		if _, _, err := s.CreateUnitType(t.Context(), in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	in := UnitTypeInput{Name: ptr(" Team "), Colour: ptr(" #AABBCC ")}
	if err := in.check(true); err != nil || *in.Name != "Team" || *in.Colour != "#aabbcc" {
		t.Errorf("tidying: %v %q %q", err, *in.Name, *in.Colour)
	}
	for name, in := range map[string]UnitInput{
		"no name":          {TypeID: ptr(uuid.New())},
		"no type":          {Name: ptr("Sales")},
		"long short name":  {TypeID: ptr(uuid.New()), Name: ptr("Sales"), ShortName: ptr(strings.Repeat("x", 21))},
		"long description": {TypeID: ptr(uuid.New()), Name: ptr("Sales"), Description: ptr(strings.Repeat("x", 4001))},
	} {
		var invalid Invalid
		if _, _, err := s.CreateUnit(t.Context(), in, nil, DayOf(time.Now())); !errors.As(err, &invalid) || invalid.Error() == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := s.UpdateUnit(t.Context(), uuid.New(), UnitInput{Name: ptr("")}, DayOf(time.Now())); err == nil {
		t.Error("an empty name was taken")
	}
	if _, _, err := s.UpdateUnitType(t.Context(), uuid.New(), UnitTypeInput{Icon: ptr("?")}); err == nil {
		t.Error("an odd icon was taken")
	}
	id := uuid.New()
	if _, _, err := s.MoveUnit(t.Context(), id, &id, DayOf(time.Now())); !errors.Is(err, ErrCycle) {
		t.Errorf("moving a unit into itself = %v", err)
	}
}

func TestTheDatabasesRefusalsAreNamed(t *testing.T) {
	for err, want := range map[error]error{
		&pgconn.PgError{Code: "23P01", ConstraintName: "unit_placement_one_root"}:       ErrRootTaken,
		&pgconn.PgError{Code: "23000", ConstraintName: "unit_placement_no_cycle"}:       ErrCycle,
		&pgconn.PgError{Code: "23503", ConstraintName: "unit_type_id_fkey"}:             ErrTypeNotFound,
		&pgconn.PgError{Code: "23503", ConstraintName: "unit_placement_parent_id_fkey"}: ErrParentNotThere,
	} {
		if got := unitError(err); !errors.Is(got, want) {
			t.Errorf("%v became %v, want %v", err, got, want)
		}
	}
	other := &pgconn.PgError{Code: "42P01"}
	if unitError(other) != error(other) || unitError(nil) != nil {
		t.Error("other errors were dressed up")
	}
	if !errors.Is(typeError(&pgconn.PgError{Code: "23505"}), ErrTypeNameTaken) || !errors.Is(typeError(&pgconn.PgError{Code: "23503"}), ErrTypeInUse) {
		t.Error("type refusals")
	}
	if !sameParent(nil, nil) || sameParent(nil, ptr(uuid.New())) {
		t.Error("sameParent")
	}
}

func TestPositionInputAndRefusals(t *testing.T) {
	s := &Service{}
	for name, in := range map[string]PositionInput{
		"no title":         {},
		"long title":       {Title: ptr(strings.Repeat("x", 201))},
		"long description": {Title: ptr("Lead"), Description: ptr(strings.Repeat("x", 4001))},
	} {
		var invalid Invalid
		if _, _, err := s.CreatePosition(t.Context(), uuid.New(), in, DayOf(time.Now())); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := s.UpdatePosition(t.Context(), uuid.New(), PositionInput{Title: ptr(" ")}, DayOf(time.Now())); err == nil {
		t.Error("an empty title was taken")
	}
	id := uuid.New()
	if _, _, err := s.SetReportsTo(t.Context(), id, &id, DayOf(time.Now())); !errors.Is(err, ErrReportsToItself) {
		t.Errorf("a solid line to itself = %v", err)
	}
	if _, _, err := s.AddDottedLine(t.Context(), id, id, DayOf(time.Now())); !errors.Is(err, ErrReportsToItself) {
		t.Errorf("a dotted line to itself = %v", err)
	}
	for constraint, want := range map[string]error{
		"reporting_line_no_cycle":           ErrReportsToItself,
		"reporting_line_once":               ErrLineExists,
		"position_one_head":                 ErrHeadTaken,
		"position_unit_id_fkey":             ErrUnitNotThere,
		"reporting_line_reports_to_id_fkey": ErrTargetNotThere,
	} {
		if got := positionError(&pgconn.PgError{ConstraintName: constraint}); !errors.Is(got, want) {
			t.Errorf("%s became %v", constraint, got)
		}
	}
	other := errors.New("boom")
	if positionError(other) != other || positionError(&pgconn.PgError{ConstraintName: "x"}) == nil {
		t.Error("other errors")
	}
	d := DayOf(time.Now())
	next := DayOf(d.Time().AddDate(0, 0, 1))
	if !livesOn(d, nil, next) || livesOn(next, nil, d) || livesOn(d, &next, next) || !livesOn(d, &next, d) {
		t.Error("livesOn")
	}
}

func TestPeopleInputKindsAndPhotos(t *testing.T) {
	s := &Service{}
	for name, in := range map[string]PersonInput{
		"no name":       {},
		"an address":    {Name: ptr("Ann"), Email: ptr("not an address")},
		"a long phone":  {Name: ptr("Ann"), Phone: ptr(strings.Repeat("1", 51))},
		"long location": {Name: ptr("Ann"), Location: ptr(strings.Repeat("x", 201))},
	} {
		var invalid Invalid
		if _, _, err := s.CreatePerson(t.Context(), in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := s.UpdatePerson(t.Context(), uuid.New(), PersonInput{Name: ptr("")}); err == nil {
		t.Error("an empty name was taken")
	}
	if _, _, err := s.UpdateMe(t.Context(), PersonInput{}); !errors.Is(err, ErrNoEntry) {
		t.Errorf("nobody's entry = %v", err)
	}
	if _, err := s.Me(t.Context()); !errors.Is(err, ErrNoEntry) {
		t.Errorf("nobody's entry = %v", err)
	}
	cleared := PersonInput{Email: ptr("  ")}
	if err := cleared.check(false); err != nil || *cleared.Email != "" {
		t.Errorf("an empty address clears: %v", err)
	}
	if got := (PersonInput{Name: ptr("x"), Phone: ptr("1")}).Fields(); strings.Join(got, ",") != "name,phone" {
		t.Errorf("fields %v", got)
	}
	sub, account := "s", uuid.New()
	if (Person{Subject: &sub}).Kind() != Realm || (Person{AccountID: &account}).Kind() != Local || (Person{}).Kind() != External {
		t.Error("kinds")
	}
	if kind, err := photoType([]byte("\x89PNG\r\n\x1a\n0000")); err != nil || kind != "image/png" {
		t.Errorf("a PNG is %q, %v", kind, err)
	}
	for _, bad := range [][]byte{{}, []byte("<svg/>"), make([]byte, MaxPhotoBytes+1)} {
		if _, err := photoType(bad); !errors.Is(err, ErrBadPhoto) {
			t.Errorf("%d bytes = %v", len(bad), err)
		}
	}
	if _, err := s.SetPhoto(t.Context(), nil, []byte("text")); !errors.Is(err, ErrBadPhoto) {
		t.Errorf("text as a photo = %v", err)
	}
	if _, err := s.SetSelfEditableFields(t.Context(), []string{"salary"}); err == nil {
		t.Error("a field that is none was allowed")
	}
	for constraint, want := range map[string]error{
		"person_email": ErrEmailTaken, "person_account_id_key": ErrAccountLinked, "person_account_id_fkey": ErrAccountMissing,
	} {
		if got := personError(&pgconn.PgError{ConstraintName: constraint}); !errors.Is(got, want) {
			t.Errorf("%s became %v", constraint, got)
		}
	}
	if !errors.Is(personError(&pgconn.PgError{Code: "42501"}), ErrNotYours) || personError(nil) != nil {
		t.Error("other refusals")
	}
}

func TestHoldingInputAndRefusals(t *testing.T) {
	s := &Service{}
	d := DayOf(time.Now())
	pos := ptr(uuid.New())
	for name, in := range map[string]HoldingInput{
		"no share":        {PositionID: pos, Share: ptr(0), From: d},
		"too much":        {PositionID: pos, Share: ptr(101), From: d},
		"ends as it does": {PositionID: pos, From: d, To: &d},
	} {
		var invalid Invalid
		if _, _, err := s.GiveHolding(t.Context(), in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, in := range []HoldingInput{{From: d}, {PositionID: pos, RoleID: ptr(uuid.New()), From: d}} {
		if _, _, err := s.GiveHolding(t.Context(), in); !errors.Is(err, ErrHoldingWhat) {
			t.Errorf("a holding of %v and %v = %v", in.PositionID, in.RoleID, err)
		}
	}
	for name, in := range map[string]RoleInput{
		"no key":        {Name: ptr("Approver")},
		"no name":       {Key: ptr("approver")},
		"a bad key":     {Key: ptr("Approver!"), Name: ptr("Approver")},
		"long text":     {Key: ptr("approver"), Name: ptr("Approver"), Description: ptr(strings.Repeat("x", 4001))},
		"an empty name": {Key: ptr("approver"), Name: ptr(" ")},
	} {
		var invalid Invalid
		if _, _, err := s.CreateRole(t.Context(), in); !errors.As(err, &invalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, _, err := s.UpdateRole(t.Context(), uuid.New(), RoleInput{Key: ptr("-x")}, d); err == nil {
		t.Error("a key starting with a dash was taken")
	}
	id := uuid.New()
	another := uuid.New()
	for name, deputies := range map[string][]uuid.UUID{"itself": {another, id}, "twice": {another, another}} {
		if _, _, err := s.SetDeputies(t.Context(), id, deputies); !errors.Is(err, ErrDeputyNotThere) {
			t.Errorf("deputies %s = %v", name, err)
		}
	}
	for constraint, want := range map[string]error{
		"role_key_key": ErrRoleKeyTaken, "position_role_once": ErrRoleCarried, "position_role_role_id_fkey": ErrRoleNotFound,
	} {
		if got := roleError(&pgconn.PgError{ConstraintName: constraint}); !errors.Is(got, want) {
			t.Errorf("%s became %v", constraint, got)
		}
	}
	if boom := errors.New("boom"); roleError(boom) != boom || roleError(&pgconn.PgError{ConstraintName: "x"}) == nil {
		t.Error("other role errors")
	}
	if _, _, err := s.SetShare(t.Context(), uuid.New(), 150); err == nil {
		t.Error("a share above the week was taken")
	}
	if _, _, err := s.CreatePosition(t.Context(), uuid.New(), PositionInput{Title: ptr("Pool"), Seats: ptr(0)}, d); err == nil {
		t.Error("a position without seats was taken")
	}
	for constraint, want := range map[string]error{"holding_once": ErrHoldingOverlaps, "holding_role_once": ErrHoldingOverlaps, "holding_within_seats": ErrSeatsTaken} {
		if got := holdingError(&pgconn.PgError{ConstraintName: constraint}); !errors.Is(got, want) {
			t.Errorf("%s became %v", constraint, got)
		}
	}
	if got := positionError(&pgconn.PgError{ConstraintName: "holding_within_seats"}); !errors.Is(got, ErrTooFewSeats) {
		t.Errorf("fewer seats than holders became %v", got)
	}
	other := errors.New("boom")
	if holdingError(other) != other || holdingError(&pgconn.PgError{ConstraintName: "x"}) == nil {
		t.Error("other errors")
	}
}

func TestWalksFollowLinesTheWayTheyAreAsked(t *testing.T) {
	solidUp, dottedDown := walkQuery(up, false), walkQuery(down, true)
	for _, want := range []string{"l.reports_to_id, l.position_id, l.kind, 1 FROM", "l.position_id = ANY ($1)", "kind = 'solid'"} {
		if !strings.Contains(solidUp, want) {
			t.Errorf("the walk up lacks %q", want)
		}
	}
	if strings.Contains(solidUp, "path") || strings.Contains(solidUp, "l.near") {
		t.Error("a walk over solid lines, which form a tree, keeps a path")
	}
	for _, want := range []string{"l.reports_to_id = ANY ($1)", "r.path || l.position_id", "l.position_id <> ALL (r.path)"} {
		if !strings.Contains(dottedDown, want) {
			t.Errorf("the walk down over dotted lines lacks %q", want)
		}
	}
	if strings.Contains(dottedDown, "kind = 'solid'") {
		t.Error("a walk over dotted lines keeps to solid ones")
	}
	if links, err := walk(t.Context(), nil, nil, DayOf(time.Now()), up, Walk{}); links != nil || err != nil {
		t.Errorf("a walk from nowhere = %v, %v", links, err)
	}
	if compareBool(true, true) != 0 || compareBool(true, false) != 1 || compareBool(false, true) != -1 {
		t.Error("compareBool")
	}
}
