package accum

import (
	"math"
	"testing"
)

const tolerance = 0.00000000000001

func eq(a, b float64) bool {
	return math.Abs(a-b) < tolerance
}

func TestAccum(t *testing.T) {
	acc := New()
	if !math.IsNaN(acc.Avg()) {
		t.Errorf("expected empty acc.Avg to return NaN, got %v", acc.Avg())
	}

	acc.Observe(1)
	if !eq(acc.Min(), 1) {
		t.Errorf("expected min to be 1, got %v", acc.Min())
	}
	if !eq(acc.Max(), 1) {
		t.Errorf("expected max to be 1, got %v", acc.Max())
	}
	if !eq(acc.First(), 1) {
		t.Errorf("expected first to be 1, got %v", acc.First())
	}
	if !eq(acc.Last(), 1) {
		t.Errorf("expected last to be 1, got %v", acc.Last())
	}
	if !eq(acc.Avg(), 1) {
		t.Errorf("expected avg to be 1, got %v", acc.Avg())
	}
	if acc.Count() != 1 {
		t.Errorf("expected count to be 1, got %v", acc.Count())
	}

	acc.Observe(2)
	if !eq(acc.Min(), 1) {
		t.Errorf("expected min to be 1, got %v", acc.Min())
	}
	if !eq(acc.Max(), 2) {
		t.Errorf("expected max to be 2, got %v", acc.Max())
	}
	if !eq(acc.First(), 1) {
		t.Errorf("expected first to be 1, got %v", acc.First())
	}
	if !eq(acc.Last(), 2) {
		t.Errorf("expected last to be 2, got %v", acc.Last())
	}
	if !eq(acc.Avg(), 1.5) {
		t.Errorf("expected avg to be 1.5, got %v", acc.Avg())
	}
	if acc.Count() != 2 {
		t.Errorf("expected count to be 2, got %v", acc.Count())
	}
	if !eq(acc.Sum(), 3) {
		t.Errorf("expected sum to be 3, got %v", acc.Sum())
	}
}

func TestAgg(t *testing.T) {
	a1 := New()
	a1.Observe(1)
	a2 := New()
	a2.Observe(2)

	acc := NewFromAccs([]*Accumulator{a1, a2})
	if !eq(acc.Min(), 1) {
		t.Errorf("expected min to be 1, got %v", acc.Min())
	}
	if !eq(acc.Max(), 2) {
		t.Errorf("expected max to be 2, got %v", acc.Max())
	}
	if !eq(acc.First(), 1) {
		t.Errorf("expected first to be 1, got %v", acc.First())
	}
	if !eq(acc.Last(), 2) {
		t.Errorf("expected last to be 2, got %v", acc.Last())
	}
	if !eq(acc.Avg(), 1.5) {
		t.Errorf("expected avg to be 1.5, got %v", acc.Avg())
	}
	if acc.Count() != 2 {
		t.Errorf("expected count to be 2, got %v", acc.Count())
	}
	if !eq(acc.Sum(), 3) {
		t.Errorf("expected sum to be 3, got %v", acc.Sum())
	}
}
