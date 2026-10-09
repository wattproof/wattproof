package main

import (
	"math"
	"testing"
)

// The daily-cycle test accepts a clean daily wave, and rejects a flat series,
// a wave too shallow, and a wave with too few full days.
func TestDailyCycle(t *testing.T) {
	wave := func(depth float64, days int) ([]float64, []bool) {
		use, present := make([]float64, hours), make([]bool, hours)
		for h := range days * day {
			use[h] = 1 - depth*(1+math.Cos(2*math.Pi*float64(h%day)/day))/2
			present[h] = true
		}
		return use, present
	}
	cases := []struct {
		name  string
		depth float64
		days  int
		want  bool
	}{
		{"deep wave", 0.5, 31, true},
		{"flat", 0, 31, false},
		{"shallow", 0.1, 31, false},
		{"six days", 0.5, 6, false},
	}
	for _, c := range cases {
		use, present := wave(c.depth, c.days)
		if got := dailyCycle(use, present); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
