package features

import "math"

// Accumulator is a type to aggregate various float values
// over a fixed time interval and provide approximate stats.
// BEWARE: There's no thread safety, no strict precision
// and no bounds checking here.
type Accumulator struct {
	cnt   int64
	sum   float64
	min   float64
	max   float64
	first float64
	last  float64
}

// New creates new empty accumulator.
// The only operation that makes sense to call on empty
// Accumulator is Observe.
func New() Accumulator {
	return Accumulator{
		sum:   math.NaN(),
		min:   math.NaN(),
		max:   math.NaN(),
		first: math.NaN(),
		last:  math.NaN(),
	}
}

// NewFromAccs creates accumulator from a slice of other accumulators.
// For example we can get approximate 1m accumulated value stats by merging
// 60 1s accumulators.
func NewFromAccs(accs []Accumulator) Accumulator {
	a := New()
	if len(accs) == 0 {
		return a
	}
	a.sum = 0
	a.first = accs[0].first
	a.max = accs[0].max
	a.min = accs[0].min
	a.last = accs[len(accs)-1].last
	for _, v := range accs {
		a.cnt += v.cnt
		a.sum += v.sum
		if v.max > a.max {
			a.max = v.max
		}
		if v.min < a.min {
			a.min = v.min
		}
	}

	return a
}

// Observe updates accumulator state with new data point.
func (a *Accumulator) Observe(v float64) {
	if math.IsNaN(a.first) {
		a.first = v
		a.last = v
		a.min = v
		a.max = v
		a.sum = v
		a.cnt = 1
		return
	}

	a.last = v
	a.sum += v
	a.cnt++

	if v > a.max {
		a.max = v
	}

	if v < a.min {
		a.min = v
	}
}

func (a *Accumulator) First() float64 {
	return a.first
}

func (a *Accumulator) Last() float64 {
	return a.last
}

func (a *Accumulator) Min() float64 {
	return a.min
}

func (a *Accumulator) Max() float64 {
	return a.max
}

func (a *Accumulator) Count() int64 {
	return a.cnt
}

func (a *Accumulator) Sum() float64 {
	return a.sum
}

func (a *Accumulator) Avg() float64 {
	if a.cnt == 0 {
		return math.NaN()
	}

	return a.sum / float64(a.cnt)
}
