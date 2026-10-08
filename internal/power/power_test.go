package power

import (
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func line(idle, peak float64) Curve {
	c, err := NewCurve([]Point{{0, idle}, {1, peak}})
	if err != nil {
		panic(err)
	}
	return c
}

func TestCurveInterpolates(t *testing.T) {
	c, err := NewCurve([]Point{{0, 100}, {0.5, 200}, {1, 400}})
	if err != nil {
		t.Fatal(err)
	}
	for load, want := range map[float64]float64{
		-1: 100, 0: 100, 0.25: 150, 0.5: 200, 0.75: 300, 1: 400, 2: 400,
	} {
		if got := c.Watts(load); !near(got, want) {
			t.Errorf("Watts(%v) = %v, want %v", load, got, want)
		}
	}
	if c.Idle() != 100 || c.Peak() != 400 {
		t.Errorf("Idle, Peak = %v, %v", c.Idle(), c.Peak())
	}
}

func TestNewCurveRejects(t *testing.T) {
	for name, pts := range map[string][]Point{
		"too few":        {{0, 1}},
		"no zero":        {{0.1, 1}, {1, 2}},
		"no one":         {{0, 1}, {0.9, 2}},
		"not increasing": {{0, 1}, {0.5, 2}, {0.5, 3}, {1, 4}},
		"negative":       {{0, -1}, {1, 2}},
	} {
		if _, err := NewCurve(pts); !errors.Is(err, ErrBadCurve) {
			t.Errorf("%s: err = %v, want ErrBadCurve", name, err)
		}
	}
}

// The worked example in docs/architecture.md: a 5-minute boot at 200 W, 120 W
// idle, 10 W off, no shutdown cost. The boot costs 24 kJ above idle, recovered
// after 24000/110 s off; with the boot, about 8.6 minutes.
func TestBreakEvenArchitectureExample(t *testing.T) {
	n := Node{
		Curve:      line(120, 300),
		OffWatts:   10,
		Transition: Transition{Boot: 5 * time.Minute, BootJoules: 5 * 60 * 200},
	}
	got, ok := n.BreakEven()
	secs := 24000.0 / 110
	want := time.Duration(secs * float64(time.Second))
	if !ok || got != want {
		t.Fatalf("BreakEven = %v, %v; want %v (≈218 s)", got, ok, want)
	}
	cycle, _ := n.MinCycle()
	if cycle != want+5*time.Minute {
		t.Fatalf("MinCycle = %v, want %v", cycle, want+5*time.Minute)
	}
}

func TestBreakEvenEdges(t *testing.T) {
	// Off draws as much as idle: switching off never pays.
	if _, ok := (Node{Curve: line(50, 100), OffWatts: 50}).BreakEven(); ok {
		t.Error("idle == off: want ok = false")
	}
	// Transitions cheaper than idling through them: any off-time pays.
	n := Node{Curve: line(100, 200), OffWatts: 10,
		Transition: Transition{Boot: time.Minute, BootJoules: 60 * 50}}
	if got, ok := n.BreakEven(); !ok || got != 0 {
		t.Errorf("cheap boot: BreakEven = %v, %v; want 0, true", got, ok)
	}
	// Shutdown energy counts like boot energy.
	n = Node{Curve: line(100, 200), OffWatts: 0,
		Transition: Transition{Shutdown: time.Minute, ShutdownJoules: 60 * 150}}
	if got, _ := n.BreakEven(); got != 30*time.Second {
		t.Errorf("shutdown: BreakEven = %v, want 30s", got)
	}
}

func TestReadCurve(t *testing.T) {
	in := "# a comment\n# cpus: 64\nload,watts\n0,50\n0.5,100\n1,180\n"
	c, cpus, err := ReadCurve(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if cpus != 64 || c.Idle() != 50 || c.Peak() != 180 || !near(c.Watts(0.75), 140) {
		t.Fatalf("cpus = %d, curve = %+v", cpus, c)
	}
	for name, bad := range map[string]string{
		"no header":  "0,1\n1,2\n",
		"bad number": "load,watts\n0,x\n1,2\n",
		"bad curve":  "load,watts\n0,1\n",
	} {
		if _, _, err := ReadCurve(strings.NewReader(bad)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// The published curves in testdata must load, and must be the measured
// values: a typo would silently change every simulation.
func TestTestdataCurves(t *testing.T) {
	for file, want := range map[string]struct {
		cpus       int
		idle, peak float64
	}{
		"specpower-dell-r7425-2018.csv": {128, 84.9, 287},
		"specpower-dell-r7725-2024.csv": {768, 138, 861},
	} {
		f, err := os.Open("../../testdata/" + file)
		if err != nil {
			t.Fatal(err)
		}
		c, cpus, err := ReadCurve(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		if cpus != want.cpus || c.Idle() != want.idle || c.Peak() != want.peak {
			t.Errorf("%s: cpus %d idle %v peak %v, want %+v", file, cpus, c.Idle(), c.Peak(), want)
		}
	}
}
