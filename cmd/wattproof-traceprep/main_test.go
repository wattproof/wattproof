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

// The trailing-window memory rule (ADR-0013) learns a spike only at the next
// daily update, holds the spike plus 15% for the 8-day window, then drops back.
// Before the first update the template's request holds.
func TestMemTrail(t *testing.T) {
	s := &service{}
	for h := range hours {
		s.present[h] = true
		s.memUse[h] = 1
		s.memReq[h] = 4
	}
	spike := 10*day + 5
	s.memUse[spike] = 3
	trail := arms(s)[10]
	check := func(h int, want float64) {
		t.Helper()
		if math.Abs(trail[h]-want) > 1e-9 {
			t.Errorf("hour %d: request %v, want %v", h, trail[h], want)
		}
	}
	check(0, 4)           // template before the first update
	check(day, 1.15)      // first update: the maximum of day 0 plus 15%
	check(spike, 1.15)    // the spike is not known in its own hour
	check(11*day-1, 1.15) // nor later that day
	check(11*day, 3*1.15) // learned at the next update
	check(18*day, 3*1.15) // still inside the 8-day window
	check(19*day, 1.15)   // the window has passed the spike
	if short := arms(s)[11][spike]; math.Abs(short-(3-1.15)) > 1e-9 {
		t.Errorf("shortfall at the spike %v, want %v", short, 3-1.15)
	}
}
