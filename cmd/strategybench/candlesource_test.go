package main

import (
	"testing"
	"time"
)

// TestSampleDay checks the seed->day mapping never returns the current
// (incomplete) day, stays within the last 2 years, and is deterministic.
func TestSampleDay(t *testing.T) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for seed := int64(0); seed < 300; seed++ {
		for attempt := 0; attempt < 5; attempt++ {
			d := sampleDay(today, seed, attempt)
			if !d.Before(today) {
				t.Fatalf("seed %d attempt %d: day %s not before today %s",
					seed, attempt, d.Format("2006-01-02"), today.Format("2006-01-02"))
			}
			back := today.Sub(d)
			if back < 24*time.Hour {
				t.Fatalf("seed %d attempt %d: day %s is the incomplete current day", seed, attempt, d)
			}
			if back > historyDays*24*time.Hour {
				t.Fatalf("seed %d attempt %d: day %s older than %d days", seed, attempt, d, historyDays)
			}
			if d.Truncate(24*time.Hour) != d {
				t.Fatalf("day %s not aligned to UTC midnight", d)
			}
		}
	}
	// Deterministic.
	if !sampleDay(today, 42, 3).Equal(sampleDay(today, 42, 3)) {
		t.Error("sampleDay is not deterministic")
	}
}
