package org

import "testing"

func TestRunsKeepOneSizeEach(t *testing.T) {
	days := everyDay([]capacityDay{
		change(t, "2026-11-02", 0.5, 2, 0.5, 1),
		change(t, "2026-11-04", 2, 2, 2, 2),
		change(t, "2026-11-05", 0, 2, 0, 0),
	}, mustDay(t, "2026-11-02"), mustDay(t, "2026-11-06"))
	got := runs(days, func(d capacityDay) float64 { return d.seats - d.filled })
	want := []run{
		{mustDay(t, "2026-11-02"), mustDay(t, "2026-11-03"), 1.5},
		{mustDay(t, "2026-11-04"), mustDay(t, "2026-11-04"), 0},
		{mustDay(t, "2026-11-05"), mustDay(t, "2026-11-06"), 2},
	}
	if len(got) != len(want) {
		t.Fatalf("runs: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestPeriodRunsJoinDaysOfOneSize(t *testing.T) {
	two := 2.0
	day := func(s string, fte float64) CapacityPeriod {
		return CapacityPeriod{From: mustDay(t, s), To: mustDay(t, s), FTE: fte, Target: &two}
	}
	got := periodRuns([]CapacityPeriod{day("2026-11-02", 1), day("2026-11-03", 1), day("2026-11-04", 2.004)},
		func(p CapacityPeriod) float64 { return *p.Target - p.FTE })
	if len(got) != 2 || got[0].size != 1 || got[0].to != mustDay(t, "2026-11-03") || got[1].size != 0 {
		t.Errorf("period runs: %+v", got)
	}
}
