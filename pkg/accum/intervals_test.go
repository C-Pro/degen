package accum

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

// Since we use low number of random samples, tolerance here should be high.
const highTolerance = 1

func eqRough(a, b float64) bool {
	return math.Abs(a-b) < highTolerance
}

func TestIntervalsRandom(t *testing.T) {
	i := NewIntervals()
	i.AddInterval("1_sec", time.Second, 15)
	i.AddInterval("15_sec", time.Second*15, 4)
	i.AddInterval("1_min", time.Minute, 15)
	i.AddInterval("15_min", time.Second*15, 4)
	i.AddInterval("1_hour", time.Hour, 1)

	expectedMin := 0.0
	expactedMax := 42.0
	expectedAvg := 21.0

	now := time.Now()
	step := time.Second / 100 // 100 observations per second

	for ts := now; ts.Before(now.Add(time.Hour * 2)); ts = ts.Add(step) {
		i.Observe(ts, rand.Float64()*42.0)
	}

	for _, interval := range i.intervals {
		for idx, acc := range interval.accs {
			if !eqRough(acc.Min(), expectedMin) {
				t.Errorf("%s[%d]: expected min to be %v, got %v",
					interval.name, idx,
					expectedMin, acc.Min())
			}
			if !eqRough(acc.Max(), expactedMax) {
				t.Errorf("%s[%d]: expected max to be %v, got %v",
					interval.name, idx,
					expactedMax, acc.Max())
			}
			if !eqRough(acc.Avg(), expectedAvg) {
				t.Errorf("%s[%d]: expected avg to be %v, got %v",
					interval.name, idx,
					expectedAvg, acc.Avg())
			}
		}
	}
}

func TestIntervalsSine(t *testing.T) {
	i := NewIntervals()
	i.AddInterval("1_sec", time.Second, 15)
	i.AddInterval("15_sec", time.Second*15, 4)
	i.AddInterval("1_min", time.Minute, 15)
	i.AddInterval("15_min", time.Second*15, 4)
	i.AddInterval("1_hour", time.Hour, 1)

	expectedMin := -10.0
	expactedMax := 10.0
	expectedAvg := 0.0

	now := time.Now()
	step := time.Second / 10 // 100 observations per second

	for ts := now; ts.Before(now.Add(time.Hour * 2)); ts = ts.Add(step) {
		v := math.Sin(float64(ts.UnixNano()) / float64(time.Second)) * 10
		i.Observe(ts, v)
	}

	for _, interval := range i.intervals {
		prevLast := math.NaN()
		for idx, acc := range interval.accs {
			if idx > 0 {
				if !eqRough(acc.First(), prevLast) {
					t.Errorf("%s[%d]: expected first to be %v, got %v",
						interval.name, idx,
						prevLast, acc.First())
				}
			}
			if !eqRough(acc.Min(), expectedMin) {
				t.Errorf("%s[%d]: expected min to be %v, got %v",
					interval.name, idx,
					expectedMin, acc.Min())
			}
			if !eqRough(acc.Max(), expactedMax) {
				t.Errorf("%s[%d]: expected max to be %v, got %v",
					interval.name, idx,
					expactedMax, acc.Max())
			}
			if !eqRough(acc.Avg(), expectedAvg) {
				t.Errorf("%s[%d]: expected avg to be %v, got %v",
					interval.name, idx,
					expectedAvg, acc.Avg())
			}
			prevLast = acc.Last()
		}
	}
}
