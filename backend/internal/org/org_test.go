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
