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
