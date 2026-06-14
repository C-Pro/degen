package main

import (
	"testing"
	"time"
)

// TestSelectDay checks: valid days are strictly before today (never the
// incomplete current day) and within the fixed addressable span; the mapping is
// stable (independent of "today", so caches never shift); and a later "today"
// only widens the valid set (old days stay valid -> caches never invalidated).
func TestSelectDay(t *testing.T) {
	today := time.Date(2026, 6, 14, 0, 0, 0, 0, time.UTC)
	spanEnd := windowStart.AddDate(0, 0, addressableDays)

	for seed := int64(0); seed < 300; seed++ {
		for attempt := 0; attempt < 8; attempt++ {
			d, valid := selectDay(seed, attempt, today)
			if d.Before(windowStart) || !d.Before(spanEnd) {
				t.Fatalf("seed %d/%d: day %s outside addressable span", seed, attempt, d)
			}
			if d.Truncate(24*time.Hour) != d {
				t.Fatalf("day %s not aligned to UTC midnight", d)
			}
			if valid && !d.Before(today) {
				t.Fatalf("seed %d/%d: valid day %s not before today", seed, attempt, d)
			}
			if !valid && d.Before(today) {
				t.Fatalf("seed %d/%d: past day %s wrongly marked invalid", seed, attempt, d)
			}
		}
	}

	// Stable: the day for (seed, attempt) does not depend on "today".
	later := today.AddDate(0, 0, 100)
	d1, _ := selectDay(42, 0, today)
	d2, _ := selectDay(42, 0, later)
	if !d1.Equal(d2) {
		t.Errorf("mapping shifted with today: %s vs %s", d1, d2)
	}
	// A day valid today stays valid 100 days later (window only grows).
	if _, v := selectDay(42, 0, today); v {
		if _, v2 := selectDay(42, 0, later); !v2 {
			t.Error("a day valid today became invalid later (cache would be lost)")
		}
	}
}
