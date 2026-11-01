package org

import (
	"testing"
	"time"
)

func mustDay(t *testing.T, s string) Day {
	t.Helper()
	d, err := ParseDay(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func change(t *testing.T, s string, fte, seats, filled float64, people int) capacityDay {
	return capacityDay{day: mustDay(t, s).Time(), fte: fte, seats: seats, filled: filled, people: people}
}

func TestEveryDayTakesTheLastChangeBeforeIt(t *testing.T) {
	days := everyDay([]capacityDay{change(t, "2026-11-02", 1, 2, 1, 1), change(t, "2026-11-05", 1.5, 2, 1.5, 2)},
		mustDay(t, "2026-11-02"), mustDay(t, "2026-11-08"))
	if len(days) != 7 {
		t.Fatalf("seven days: %d", len(days))
	}
	for i, want := range []float64{1, 1, 1, 1.5, 1.5, 1.5, 1.5} {
		if days[i].fte != want || !days[i].day.Equal(mustDay(t, "2026-11-02").Time().AddDate(0, 0, i)) {
			t.Errorf("day %d: %+v", i, days[i])
		}
	}
}

func TestPeriodsAreTheMeanOfTheirWorkingDays(t *testing.T) {
	// Monday 2 November to Sunday 15 November 2026: 1 FTE through Wednesday the 4th, 2 from Thursday, 5 at weekends,
	// which a week does not count.
	var days []capacityDay
	for d := mustDay(t, "2026-11-02").Time(); !d.After(mustDay(t, "2026-11-15").Time()); d = d.AddDate(0, 0, 1) {
		fte := 1.0
		if d.Day() >= 5 {
			fte = 2
		}
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			fte = 5
		}
		days = append(days, capacityDay{day: d, fte: fte, seats: 3, filled: min(fte, 3), people: int(fte)})
	}
	weeks := periodsOf(days, ByWeek, true)
	if len(weeks) != 2 || weeks[0].From.String() != "2026-11-02" || weeks[0].To.String() != "2026-11-08" || weeks[1].From.String() != "2026-11-09" {
		t.Fatalf("two weeks from Monday: %+v", weeks)
	}
	if weeks[0].FTE != 1.4 || *weeks[0].Seats != 3 || *weeks[0].Filled != 1.4 || *weeks[0].Vacant != 1.6 || weeks[0].People != 2 {
		t.Errorf("the first week: %+v", weeks[0])
	}
	if weeks[1].FTE != 2 {
		t.Errorf("the second week: %+v", weeks[1])
	}
	if each := periodsOf(days, ByDay, false); len(each) != 14 || each[5].FTE != 5 || each[5].Seats != nil {
		t.Errorf("by day every day counts, a Saturday too: %+v", each[5])
	}
	months := periodsOf(days, ByMonth, false)
	if len(months) != 1 || months[0].From.String() != "2026-11-02" || months[0].To.String() != "2026-11-15" || months[0].FTE != 1.7 {
		t.Errorf("a month cut by the range: %+v", months)
	}
}

func TestAPeriodOfOnlyAWeekendCountsItsDays(t *testing.T) {
	weekend := []capacityDay{change(t, "2026-11-07", 1, 0, 0, 1), change(t, "2026-11-08", 2, 0, 0, 2)}
	if got := periodsOf(weekend, ByWeek, false); len(got) != 1 || got[0].FTE != 1.5 || got[0].People != 2 {
		t.Errorf("a weekend alone: %+v", got)
	}
}

func TestAPeriodsTargetIsTheMeanOfTheDaysThatHaveOne(t *testing.T) {
	two, four := 2.0, 4.0
	days := []capacityDay{
		{day: mustDay(t, "2026-11-02").Time()},
		{day: mustDay(t, "2026-11-03").Time(), target: &two},
		{day: mustDay(t, "2026-11-04").Time(), target: &four},
	}
	if got := periodsOf(days, ByWeek, false); len(got) != 1 || got[0].Target == nil || *got[0].Target != 3 {
		t.Errorf("the week's target: %+v", got)
	}
	if got := periodsOf(days[:1], ByWeek, false); got[0].Target != nil {
		t.Errorf("a week without a target: %+v", got[0])
	}
}
