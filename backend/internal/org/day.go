// Package org is the organisation model: units, positions, people, holdings and roles, every relation dated.
package org

import (
	"encoding/json"
	"fmt"
	"time"
)

// Day is a calendar day, as relations start and end on. It is written 2006-01-02 and held at midnight UTC.
type Day struct{ t time.Time }

const dayLayout = "2006-01-02"

// DayOf is the day t falls on, in t's own zone.
func DayOf(t time.Time) Day {
	y, m, d := t.Date()
	return Day{time.Date(y, m, d, 0, 0, 0, 0, time.UTC)}
}

// ParseDay reads a day written 2006-01-02.
func ParseDay(s string) (Day, error) {
	t, err := time.Parse(dayLayout, s)
	if err != nil {
		return Day{}, fmt.Errorf("%q is not a day; write it as 2006-01-02", s)
	}
	return Day{t}, nil
}

// Time is midnight UTC of the day, as the database takes a date.
func (d Day) Time() time.Time { return d.t }

func (d Day) String() string { return d.t.Format(dayLayout) }

// IsZero says whether d is no day at all.
func (d Day) IsZero() bool { return d.t.IsZero() }

// Before says whether d comes before other.
func (d Day) Before(other Day) bool { return d.t.Before(other.t) }

func (d Day) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func (d *Day) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return fmt.Errorf("a day is a string written 2006-01-02")
	}
	parsed, err := ParseDay(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// dayPtr reads a nullable date column.
func dayPtr(t *time.Time) *Day {
	if t == nil {
		return nil
	}
	d := DayOf(*t)
	return &d
}
