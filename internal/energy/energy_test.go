package energy

import (
	"errors"
	"math"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC)

func at(sec float64) time.Time {
	return t0.Add(time.Duration(sec * float64(time.Second)))
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestFromPowerConstant(t *testing.T) {
	// 100 W for 10 s, sampled every second: exactly 1000 J.
	var pts []Point
	for s := 0; s <= 10; s++ {
		pts = append(pts, Point{T: at(float64(s)), V: 100})
	}
	r, err := FromPower(pts, MaxGap)
	if err != nil {
		t.Fatal(err)
	}
	if !near(r.Joules, 1000) {
		t.Fatalf("Joules = %v, want 1000", r.Joules)
	}
	if r.Span != 10*time.Second || len(r.Gaps) != 0 {
		t.Fatalf("Span = %v, Gaps = %v", r.Span, r.Gaps)
	}
}

func TestFromPowerRamp(t *testing.T) {
	// Linear ramp 0→100 W over 10 s: the trapezoid rule is exact, 500 J.
	var pts []Point
	for s := 0; s <= 10; s++ {
		pts = append(pts, Point{T: at(float64(s)), V: float64(s) * 10})
	}
	r, err := FromPower(pts, MaxGap)
	if err != nil {
		t.Fatal(err)
	}
	if !near(r.Joules, 500) {
		t.Fatalf("Joules = %v, want 500", r.Joules)
	}
}

func TestFromPowerGapNotInterpolated(t *testing.T) {
	// 100 W at 0 s and 1 s, then nothing until 7 s: the 6 s interval is a gap.
	pts := []Point{{at(0), 100}, {at(1), 100}, {at(7), 100}, {at(8), 100}}
	r, err := FromPower(pts, MaxGap)
	if err != nil {
		t.Fatal(err)
	}
	if !near(r.Joules, 200) {
		t.Fatalf("Joules = %v, want 200 (gap must contribute nothing)", r.Joules)
	}
	if len(r.Gaps) != 1 || r.Gaps[0].Duration() != 6*time.Second {
		t.Fatalf("Gaps = %v, want one 6s gap", r.Gaps)
	}
	if !near(r.GapFraction(), 6.0/8.0) {
		t.Fatalf("GapFraction = %v, want 0.75", r.GapFraction())
	}
}

func TestFromPowerGapBoundary(t *testing.T) {
	// An interval of exactly MaxGap is still integrated; one nanosecond more is not.
	exact := []Point{{at(0), 100}, {t0.Add(MaxGap), 100}}
	r, _ := FromPower(exact, MaxGap)
	if len(r.Gaps) != 0 || !near(r.Joules, 500) {
		t.Fatalf("exact MaxGap: Joules = %v, Gaps = %v", r.Joules, r.Gaps)
	}
	over := []Point{{at(0), 100}, {t0.Add(MaxGap + time.Nanosecond), 100}}
	r, _ = FromPower(over, MaxGap)
	if len(r.Gaps) != 1 || r.Joules != 0 {
		t.Fatalf("over MaxGap: Joules = %v, Gaps = %v", r.Joules, r.Gaps)
	}
}

func TestFromPowerRejectsBadTimes(t *testing.T) {
	for name, pts := range map[string][]Point{
		"duplicate": {{at(0), 1}, {at(0), 1}},
		"backwards": {{at(1), 1}, {at(0), 1}},
	} {
		if _, err := FromPower(pts, MaxGap); !errors.Is(err, ErrNotIncreasing) {
			t.Errorf("%s: err = %v, want ErrNotIncreasing", name, err)
		}
	}
}

func TestFromPowerTooFewPoints(t *testing.T) {
	for _, pts := range [][]Point{nil, {{at(0), 100}}} {
		r, err := FromPower(pts, MaxGap)
		if err != nil || r.Joules != 0 || r.Span != 0 {
			t.Fatalf("len %d: r = %+v, err = %v", len(pts), r, err)
		}
	}
}

func TestFromCounterExactAcrossGap(t *testing.T) {
	// The counter difference is exact even across a gap; the gap is still reported.
	pts := []Point{{at(0), 1000}, {at(1), 1100}, {at(20), 3000}}
	r, err := FromCounter(pts, MaxGap)
	if err != nil {
		t.Fatal(err)
	}
	if !near(r.Joules, 2000) {
		t.Fatalf("Joules = %v, want 2000", r.Joules)
	}
	if len(r.Gaps) != 1 || r.Gaps[0].Duration() != 19*time.Second {
		t.Fatalf("Gaps = %v, want one 19s gap", r.Gaps)
	}
}

func TestFromCounterDecrease(t *testing.T) {
	pts := []Point{{at(0), 1000}, {at(1), 900}}
	if _, err := FromCounter(pts, MaxGap); !errors.Is(err, ErrCounterDecreased) {
		t.Fatalf("err = %v, want ErrCounterDecreased", err)
	}
}
