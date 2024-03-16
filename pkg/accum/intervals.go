package accum

import (
	"sort"
	"time"
)

type interval struct {
	name       string
	duration   time.Duration
	accs       []*Accumulator
	lastAccIdx int
}

// Intervals allows to merge multiple accumulators over time intervals into
// accumulators over lager intervals in incremental manner.
// E.g. given these time intervals:
// "1_sec":  time.Second
// "15_sec": time.Second * 15
// "1_min":  time.Minute
// "15_min": time.Minute * 15
// "1_hour": time.Hour
// We an have 15 seconds interval accumulator, and then merge it into 1 minute
// interval accumulator, and then merge it into 15 minutes interval accumulator, and so on.
type Intervals struct {
	intervals []interval
}

// NewIntervals creates a new Intervals instance.
func NewIntervals() *Intervals {
	return &Intervals{}
}

// AddInterval adds a new interval to the Intervals instance.
// It requires name, duration and count of accumulators.
func (i *Intervals) AddInterval(name string, duration time.Duration, count int) {
	accs := make([]*Accumulator, count)
	for i := range accs {
		accs[i] = New()
	}
	i.intervals = append(i.intervals, interval{name, duration, accs, -1})
	// In case intervals are added in random order, we need to sort them by duration.
	sort.Slice(i.intervals, func(a, b int) bool {
		return i.intervals[a].duration < i.intervals[b].duration
	})
}

// getAccIdx returns index of the accumulator in the interval for the given time.
// It tracks last used accumulator index and resets it when time interval changes.
func (i *Intervals) getAccIdx(now time.Time, intrvlIdx int) int {
	intrvl := i.intervals[intrvlIdx]
	idx := int(time.Duration(now.UnixNano()) %
		(intrvl.duration * time.Duration(len(intrvl.accs))) /
		intrvl.duration)

	// When idx changes, reset the accumulator.
	if intrvl.lastAccIdx != idx {
		i.intervals[intrvlIdx].accs[idx].Reset()
		i.intervals[intrvlIdx].lastAccIdx = idx
	}

	return idx
}

// Observe adds new observation to lowest of the interval accumulators.
// It selects specific accumulator based on the duration of the interval and current time.
// Once observation added, it propagates changes to higher interval accumulators (only current ones).
func (i *Intervals) Observe(now time.Time, v float64) {
	idx := i.getAccIdx(now, 0)
	i.intervals[0].accs[idx].Observe(v)
	for j := 1; j < len(i.intervals); j++ {
		idx := i.getAccIdx(now, j)
		i.intervals[j].accs[idx] = NewFromAccs(i.intervals[j-1].accs)
	}
}


// GetValues returns all aggregated values one accumulator per interval.
// It returnes last "closed one", or the one before current.
func (i *Intervals) GetValues() map[string]map[string]float64 {
	vals := make(map[string]map[string]float64, len(i.intervals))
	for j, ci := range i.intervals {
		idx := (i.intervals[j].lastAccIdx + len(i.intervals[j].accs) - 1) % len(i.intervals[j].accs)
		vals[ci.name] = i.intervals[j].accs[idx].Values()
	}
	return vals
}
