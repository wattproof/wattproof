package sim

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wattproof/wattproof/internal/power"
)

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(b)) }

func curve(idle, peak float64) power.Curve {
	c, err := power.NewCurve([]power.Point{{Load: 0, Watts: idle}, {Load: 1, Watts: peak}})
	if err != nil {
		panic(err)
	}
	return c
}

// node is a worker with a linear curve, 10 W off, and a boot of the given
// length drawing peak power.
func node(name string, cpus, idle, peak float64, boot time.Duration) NodeSpec {
	return NodeSpec{Name: name, CPUs: cpus, Power: power.Node{
		Curve: curve(idle, peak), OffWatts: 10,
		Transition: power.Transition{
			Shutdown: time.Minute, ShutdownJoules: 60 * idle,
			Boot: boot, BootJoules: boot.Seconds() * peak,
		},
	}}
}

func flat(v float64, n int) []float64 {
	s := make([]float64, n)
	for i := range s {
		s[i] = v
	}
	return s
}

func arms(demand []float64) []Policy {
	return []Policy{AlwaysOn{}, NewAutoscaler(), NewPlanner(Oracle{Series: demand}),
		NewPlanner(SeasonalNaive{Period: 24 * 60})}
}

// B0's energy is the integral of its curves at the spread load, computed here
// independently of Run.
func TestAlwaysOnEnergyIsIntegralOfCurves(t *testing.T) {
	nodes := []NodeSpec{node("a", 10, 100, 300, time.Minute), node("b", 30, 50, 250, time.Minute)}
	demand := []float64{0, 8, 20, 40, 12}
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: 0.5, BaseWatts: 7}
	r, err := Run(cfg, AlwaysOn{})
	if err != nil {
		t.Fatal(err)
	}
	var want float64
	for _, d := range demand {
		load := d / 40 * 0.5 // spread: same fraction on both nodes
		want += 60 * (7 + (100 + 200*load) + (50 + 200*load))
	}
	if !near(r.Joules, want) {
		t.Fatalf("Joules = %v, want %v", r.Joules, want)
	}
	if r.PowerOffs != 0 || r.NodeHours != 2*5.0/60 {
		t.Fatalf("PowerOffs = %d, NodeHours = %v", r.PowerOffs, r.NodeHours)
	}
}

// Under constant full demand nothing can be switched off, so every arm uses
// exactly B0's energy.
func TestFullDemandEveryArmEqualsB0(t *testing.T) {
	nodes := []NodeSpec{node("a", 10, 100, 300, 5*time.Minute), node("b", 10, 100, 300, 5*time.Minute)}
	demand := flat(20, 600)
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: 1, MinNodes: 1}
	var b0 float64
	for _, p := range arms(demand) {
		r, err := Run(cfg, p)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name() == "B0" {
			b0 = r.Joules
		}
		if r.Joules != b0 || r.PowerOffs != 0 || r.ShortfallMinutes != 0 {
			t.Errorf("%s: Joules = %v (B0 %v), PowerOffs = %d, Shortfall = %v",
				p.Name(), r.Joules, b0, r.PowerOffs, r.ShortfallMinutes)
		}
	}
}

// A dip shorter than the break-even cycle: B1 waits its 10 minutes and then
// cycles a node anyway; T sees the dip end in the forecast and keeps it on.
func TestPlannerSkipsDipShorterThanBreakEven(t *testing.T) {
	boot := 10 * time.Minute // drawing peak: 30 kJ above idle
	nodes := []NodeSpec{node("a", 10, 100, 150, boot), node("b", 10, 100, 150, boot)}
	// An 18-minute dip: longer than B1's 10 minutes unneeded, shorter than
	// T's break-even cycle plus its 5-minute margin.
	dip := 18
	if c, _ := nodes[0].Power.MinCycle(); time.Duration(dip)*time.Minute >= c+5*time.Minute {
		t.Fatalf("dip of %d minutes is not shorter than the cycle %v plus margin", dip, c)
	}
	demand := append(append(flat(15, 60), flat(2, dip)...), flat(15, 60)...)
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: 1, MinNodes: 1}

	t1, _ := Run(cfg, NewPlanner(Oracle{Series: demand}))
	b1, _ := Run(cfg, NewAutoscaler())
	if t1.PowerOffs != 0 {
		t.Errorf("T powered off %d times during a dip shorter than break-even", t1.PowerOffs)
	}
	if b1.PowerOffs == 0 {
		t.Errorf("B1 did not power off; the dip no longer tests anything")
	}
}

// On mixed hardware at low demand, T keeps the efficient node and powers the
// other off. B1 chooses by node order, so here it keeps the wrong one.
func TestPlannerKeepsEfficientNode(t *testing.T) {
	nodes := []NodeSpec{
		node("old", 10, 100, 300, 2*time.Minute), // 20 W per CPU at half load
		node("new", 10, 50, 150, 2*time.Minute),  // 10 W per CPU at half load
	}
	demand := flat(3, 240)
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: 1, MinNodes: 1}
	r, _ := Run(cfg, NewPlanner(Oracle{Series: demand}))
	b1, _ := Run(cfg, NewAutoscaler())
	if r.PowerOffs != 1 || r.PoweredHours[0] >= r.PoweredHours[1] {
		t.Fatalf("T: PowerOffs = %d, powered hours old/new = %v", r.PowerOffs, r.PoweredHours)
	}
	if b1.PoweredHours[0] <= b1.PoweredHours[1] {
		t.Fatalf("B1 kept the efficient node; the test no longer contrasts them: %v", b1.PoweredHours)
	}
}

// reckless asks to power off every node at every step.
type reckless struct{}

func (reckless) Name() string     { return "reckless" }
func (reckless) Packing() Packing { return Spread }
func (reckless) Decide(s *State) (on, off []int) {
	for i := range s.Nodes {
		off = append(off, i)
	}
	return nil, off
}

// The simulator refuses power-offs that break invariants 1 and 3, whatever the
// policy asks for.
func TestRunEnforcesInvariants(t *testing.T) {
	nodes := []NodeSpec{node("a", 10, 100, 300, time.Minute), node("b", 10, 100, 300, time.Minute),
		node("c", 10, 100, 300, time.Minute)}
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: flat(15, 30), UseRatio: 1, MinNodes: 1}
	r, err := Run(cfg, reckless{})
	if err != nil {
		t.Fatal(err)
	}
	// Demand 15 needs two 10-CPU nodes: one may go, the other two must stay.
	if r.PowerOffs != 1 || r.Rejected == 0 || r.MinPowered != 2 || r.ShortfallMinutes != 0 {
		t.Fatalf("PowerOffs = %d, Rejected = %d, MinPowered = %d, Shortfall = %v",
			r.PowerOffs, r.Rejected, r.MinPowered, r.ShortfallMinutes)
	}
	cfg.Demand = flat(0, 30)
	if r, _ = Run(cfg, reckless{}); r.MinPowered != 1 {
		t.Fatalf("no demand: MinPowered = %d, want MinNodes 1", r.MinPowered)
	}
}

// On the real diurnal curve, no arm breaks invariants 1 and 3, and T with a
// perfect forecast never leaves pods pending.
func TestDiurnalInvariants(t *testing.T) {
	f, err := os.Open("../../testdata/demand-dewiki-2025-09-week.csv")
	if err != nil {
		t.Fatal(err)
	}
	shape, err := ReadShape(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	week := Interpolate(shape, 60)
	var nodes []NodeSpec
	for i := range 6 {
		nodes = append(nodes, node(string(rune('a'+i)), 100, 120, 400, 5*time.Minute))
	}
	demand := make([]float64, 0, 2*len(week))
	for _, s := range append(week, week...) {
		demand = append(demand, 0.7*600*s)
	}
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: 0.5, MinNodes: 1, Score: len(week)}
	var b0 float64
	for _, p := range arms(demand) {
		r, err := Run(cfg, p)
		if err != nil {
			t.Fatal(err)
		}
		if p.Name() == "B0" {
			b0 = r.Joules
		}
		if r.Rejected != 0 || r.MinPowered < cfg.MinNodes {
			t.Errorf("%s: Rejected = %d, MinPowered = %d", p.Name(), r.Rejected, r.MinPowered)
		}
		if r.Joules > b0 {
			t.Errorf("%s uses more energy than B0: %v > %v", p.Name(), r.Joules, b0)
		}
	}
	r, _ := Run(cfg, NewPlanner(Oracle{Series: demand}))
	if r.ShortfallMinutes != 0 {
		t.Errorf("T with a perfect forecast left pods pending for %v minutes", r.ShortfallMinutes)
	}
}

func TestSeasonalNaive(t *testing.T) {
	f := SeasonalNaive{Period: 3}
	if got := f.Forecast([]float64{5, 6}, 1); got != 6 {
		t.Errorf("short history: %v, want 6 (latest value)", got)
	}
	// Today runs at twice yesterday's level: 1,2,3 then 2,4,6.
	h := []float64{1, 2, 3, 2}
	if got := f.Forecast(h, 1); got != 4 {
		t.Errorf("Forecast(h, 1) = %v, want 4", got)
	}
}

func TestReadShapeAndInterpolate(t *testing.T) {
	v, err := ReadShape(strings.NewReader("# c\nhour,views\n1,50\n2,100\n"))
	if err != nil || len(v) != 2 || v[0] != 0.5 || v[1] != 1 {
		t.Fatalf("ReadShape = %v, %v", v, err)
	}
	got := Interpolate([]float64{0, 1}, 2)
	want := []float64{0, 0.5, 1, 0.5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Interpolate = %v, want %v", got, want)
		}
	}
}
