package accum

import (
	"math"
	"math/rand"
	"testing"
	"time"
)

const (
	// Since we use low number of random samples, tolerance here should be high.
	// We can decrease the tolerance, but then we need to increase the number of samples
	// which will make tests slow in ci.
	// Otherwise we will get flaky tests.
	highTolerance = 2.0
	// With 500 samples per second TestIntervalsRandom takes < 2s on my machine.
	// Test emulates 2 hours of observations, so total number of samples will be
	// 2 * 60 * 60 * 500 = 3 600 000 or 1.8M observations per second.
	samplesPerSecond = 500
)

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
	step := time.Second / samplesPerSecond

	for ts := now; ts.Before(now.Add(time.Hour * 2)); ts = ts.Add(step) {
		i.Observe(ts, rand.Float64()*42.0)
	}

	for _, interval := range i.intervals {
		for idx, acc := range interval.accs {
			if interval.name == "1_sec" && acc.Count() < samplesPerSecond {
				// Current 1 second interval will contain small number of samples
				// and have large deviation, so we skip it as very flaky.
				continue
			}
			if !eqRough(acc.Min(), expectedMin) {
				t.Logf("%#v", acc)
				t.Errorf("%s[%d]: expected min to be %v, got %v",
					interval.name, idx,
					expectedMin, acc.Min())
			}
			if !eqRough(acc.Max(), expactedMax) {
				t.Logf("%#v", acc)
				t.Errorf("%s[%d]: expected max to be %v, got %v",
					interval.name, idx,
					expactedMax, acc.Max())
			}
			if !eqRough(acc.Avg(), expectedAvg) {
				t.Logf("%#v", acc)
				t.Errorf("%s[%d]: expected avg to be %v, got %v",
					interval.name, idx,
					expectedAvg, acc.Avg())
			}
		}
	}
}

// TestIntervalsSine checks that adjacent intervals are consistent with each other.
// It uses continuous function (sin) to generate data points, so we expect adjacent
// intervals first and last values to be close to each other.
func TestIntervalsSine(t *testing.T) {
	i := NewIntervals()
	i.AddInterval("1_sec", time.Second, 15)
	i.AddInterval("15_sec", time.Second*15, 4)
	i.AddInterval("1_min", time.Minute, 15)
	i.AddInterval("15_min", time.Second*15, 4)
	i.AddInterval("1_hour", time.Hour, 1)

	now := time.Now()
	step := time.Second / 10 // 100 observations per second

	for ts := now; ts.Before(now.Add(time.Hour * 2)); ts = ts.Add(step) {
		v := math.Sin(float64(ts.UnixNano())/float64(time.Second)) * 10
		i.Observe(ts, v)
	}

	for _, interval := range i.intervals {
		prevFirst := math.NaN()
		for n := 0; n < len(interval.accs); n++ {
			idx := (interval.lastAccIdx + len(interval.accs) - n) % len(interval.accs)
			acc := interval.accs[idx]

			if idx == interval.lastAccIdx {
				prevFirst = acc.First()
				continue
			}

			if !eqRough(acc.Last(), prevFirst) {
				t.Errorf("%s[%d]: expected last to be %v, got %v",
					interval.name, idx,
					prevFirst, acc.Last())
			}

			prevFirst = acc.first
		}
	}
}

// cpu: Intel(R) Core(TM) i5-8250U CPU @ 1.60GHz
// BenchmarkObserve-8       2348199               521.0 ns/op           240 B/op          5 allocs/op
func BenchmarkObserve(b *testing.B) {
	i := NewIntervals()
	i.AddInterval("1_sec", time.Second, 15)
	i.AddInterval("15_sec", time.Second*15, 4)
	i.AddInterval("1_min", time.Minute, 15)
	i.AddInterval("15_min", time.Second*15, 4)
	i.AddInterval("1_hour", time.Hour, 24)
	i.AddInterval("1_day", time.Hour*24, 1)

	b.ResetTimer()

	for j := 0; j < b.N; j++ {
		i.Observe(time.Now(), float64(j))
	}
}
