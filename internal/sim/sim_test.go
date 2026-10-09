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

func TestShapeRequests(t *testing.T) {
	usage := []float64{1, 3, 2, 0.1, 0.2, 0.1, 8, 9}
	got := ShapeRequests(usage, 3, 10, 0.5, 0.5)
	want := []float64{4.5, 4.5, 4.5, 0.5, 0.5, 0.5, 10, 10}
	for i := range want {
		if !near(got[i], want[i]) {
			t.Fatalf("ShapeRequests = %v, want %v", got, want)
		}
	}
	// Below the template's request, shaped requests always cover usage plus
	// the margin.
	for i, u := range usage {
		if got[i] < 10 && got[i] < u*1.5-1e-9 {
			t.Errorf("step %d: request %v below usage %v plus margin", i, got[i], u)
		}
	}
}

func TestVPATarget(t *testing.T) {
	usage := []float64{10, 1, 9, 2, 8, 3, 7, 4, 6, 5}
	if got := VPATarget(usage, 0.9, 0.15); !near(got, 11.5) {
		t.Fatalf("VPATarget = %v, want 11.5 (p90 = 10, plus 15%%)", got)
	}
}

// With Usage given, each node runs usage in proportion to its requests, and a
// node whose share exceeds its capacity counts as overloaded.
func TestUsageSeparateFromRequests(t *testing.T) {
	nodes := []NodeSpec{node("a", 10, 100, 300, time.Minute), node("b", 10, 100, 300, time.Minute)}
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: []float64{20, 10, 4},
		Usage: []float64{5, 10, 30}}
	r, err := Run(cfg, AlwaysOn{})
	if err != nil {
		t.Fatal(err)
	}
	// Spread: each node holds half the usage. Loads 0.25, 0.5, then 1.5 (clamped to 1).
	want := 60 * 2 * ((100 + 200*0.25) + (100 + 200*0.5) + 300)
	if !near(r.Joules, want) || r.OverloadMinutes != 1 {
		t.Fatalf("Joules = %v (want %v), OverloadMinutes = %v (want 1)", r.Joules, want, r.OverloadMinutes)
	}
	cfg.Usage = []float64{1}
	if _, err := Run(cfg, AlwaysOn{}); err == nil {
		t.Fatal("Usage of the wrong length: want an error")
	}
}

// A service with fixed replicas keeps its requests all night while its usage
// falls. Shaping its requests lets T power more nodes off, without pending
// pods or throttling.
func TestResizeLetsPlannerSaveMore(t *testing.T) {
	var nodes []NodeSpec
	for i := range 4 {
		nodes = append(nodes, node(string(rune('a'+i)), 10, 100, 300, 2*time.Minute))
	}
	var usage []float64
	for range 4 { // two days, half busy and half quiet, twice
		usage = append(append(usage, flat(12, 12*60)...), flat(1, 12*60)...)
	}
	fixed := flat(30, len(usage))
	shaped := ShapeRequests(usage, 60, 30, 0.2, 1)
	run := func(req []float64) Result {
		cfg := Config{Step: time.Minute, Nodes: nodes, Demand: req, Usage: usage, MinNodes: 1, Score: len(usage) / 2}
		r, err := Run(cfg, NewPlanner(Oracle{Series: req}))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	plain, resized := run(fixed), run(shaped)
	if !(resized.Joules < plain.Joules) || resized.PowerOffs == 0 {
		t.Fatalf("resized Joules %v not below plain %v (power-offs %d)", resized.Joules, plain.Joules, resized.PowerOffs)
	}
	if resized.ShortfallMinutes != 0 || resized.OverloadMinutes != 0 || resized.Rejected != 0 {
		t.Fatalf("resized: shortfall %v, overload %v, rejected %d",
			resized.ShortfallMinutes, resized.OverloadMinutes, resized.Rejected)
	}
}

// withMem gives each node memory equal to its CPUs.
func withMem(nodes []NodeSpec) []NodeSpec {
	for i := range nodes {
		nodes[i].Mem = nodes[i].CPUs
	}
	return nodes
}

// The trace analysis plan's exit test: with memory requests at 100% of
// capacity, lowering CPU requests frees no server, so no arm powers one off.
// Without memory, the same CPU demand lets T and B1 power one off.
func TestFullMemoryFreesNoServer(t *testing.T) {
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, 2*time.Minute), node("b", 10, 100, 300, 2*time.Minute)})
	demand := flat(4, 240) // CPU alone would fit on one node
	for _, mem := range [][]float64{nil, flat(20, 240)} {
		cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, MemDemand: mem, UseRatio: 1, MinNodes: 1}
		for _, p := range arms(demand) {
			r, err := Run(cfg, p)
			if err != nil {
				t.Fatal(err)
			}
			switch {
			case mem != nil && (r.PowerOffs != 0 || r.ShortfallMinutes != 0 || r.Rejected != 0):
				t.Errorf("%s with full memory: PowerOffs = %d, Shortfall = %v, Rejected = %d",
					p.Name(), r.PowerOffs, r.ShortfallMinutes, r.Rejected)
			case mem == nil && p.Name() != "B0" && r.PowerOffs == 0:
				t.Errorf("%s without memory powered nothing off; the test no longer contrasts", p.Name())
			}
		}
	}
}

// Memory beyond the Ready nodes' capacity leaves pods pending, even when
// their CPU would fit.
func TestMemoryShortfall(t *testing.T) {
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, time.Minute)})
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: flat(2, 5), MemDemand: flat(12, 5), UseRatio: 1}
	r, err := Run(cfg, AlwaysOn{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ShortfallMinutes != 5 {
		t.Fatalf("ShortfallMinutes = %v, want 5", r.ShortfallMinutes)
	}
	if _, err := Run(Config{Step: time.Minute, Nodes: []NodeSpec{node("a", 10, 100, 300, time.Minute)},
		Demand: flat(2, 5), MemDemand: flat(1, 5)}, AlwaysOn{}); err == nil {
		t.Fatal("a node without memory was accepted for a run with memory demand")
	}
}

// Placement is limited by whichever resource runs out first: with pods
// asking 2 units of memory per CPU, a node with 10 CPUs and 10 memory holds
// only 5 CPUs of requests.
func TestPlaceUsesEffectiveCapacity(t *testing.T) {
	nodes := []Node{{Spec: NodeSpec{CPUs: 10, Mem: 10}, Phase: On}, {Spec: NodeSpec{CPUs: 10, Mem: 10}, Phase: On}}
	req := Place(nodes, 8, 2, Pack)
	if !near(req[0], 5) || !near(req[1], 3) {
		t.Fatalf("Pack = %v, want [5 3]", req)
	}
	req = Place(nodes, 8, 2, Spread)
	if !near(req[0], 4) || !near(req[1], 4) {
		t.Fatalf("Spread = %v, want [4 4]", req)
	}
}

// The simulator refuses power-offs that would leave memory with nowhere to
// go, whatever the policy asks for.
func TestRunEnforcesMemory(t *testing.T) {
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, time.Minute), node("b", 10, 100, 300, time.Minute),
		node("c", 10, 100, 300, time.Minute)})
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: flat(3, 60), MemDemand: flat(15, 60), UseRatio: 1, MinNodes: 1}
	r, err := Run(cfg, reckless{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ShortfallMinutes != 0 || r.MinPowered != 2 || r.Rejected == 0 {
		t.Fatalf("Shortfall = %v, MinPowered = %d, Rejected = %d", r.ShortfallMinutes, r.MinPowered, r.Rejected)
	}
}

// B1 judges a node by the larger of its CPU and memory utilisation, as
// Cluster Autoscaler does: little CPU but much memory is not unneeded.
func TestAutoscalerCountsMemory(t *testing.T) {
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, time.Minute), node("b", 10, 100, 300, time.Minute)})
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: flat(6, 120), MemDemand: flat(14, 120), UseRatio: 1, MinNodes: 1}
	r, _ := Run(cfg, NewAutoscaler())
	if r.PowerOffs != 0 || r.Rejected != 0 {
		t.Fatalf("B1 powered off %d nodes and asked for %d refused ones while memory needed both",
			r.PowerOffs, r.Rejected)
	}
}

// When memory requests rise after B1 powered a node off, B1 powers it on
// again: pending memory counts, not only pending CPU. The pods wait only for
// the boot.
func TestAutoscalerScalesUpForMemory(t *testing.T) {
	boot := 2 * time.Minute
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, boot), node("b", 10, 100, 300, boot)})
	demand := flat(4, 120)
	mem := append(flat(4, 60), flat(15, 60)...)
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: demand, MemDemand: mem, UseRatio: 1, MinNodes: 1}
	r, _ := Run(cfg, NewAutoscaler())
	if r.PowerOffs == 0 {
		t.Fatal("B1 never powered a node off; the test no longer tests scale-up")
	}
	if r.ShortfallMinutes > boot.Minutes()+2 {
		t.Fatalf("pods pending for %v minutes, more than the boot", r.ShortfallMinutes)
	}
}

// Shaping learned from the past: the first season keeps the template; later
// seasons use the highest peak seen in the same period before, plus margin.
// Changing the future never changes the past.
func TestShapeLearned(t *testing.T) {
	peak := []float64{1, 2, 3, 4, 2, 2, 2, 2, 9, 0, 0, 0}
	orig, floor := flat(10, 12), flat(0.5, 12)
	got := ShapeLearned(peak, orig, floor, 1, 4, 0.5)
	want := []float64{10, 10, 10, 10, 1.5, 3, 4.5, 6, 3, 3, 4.5, 6}
	for i := range want {
		if !near(got[i], want[i]) {
			t.Fatalf("step %d: %v, want %v (all %v)", i, got[i], want[i], got)
		}
	}
	peak[11] = 100 // the future
	if again := ShapeLearned(peak, orig, floor, 1, 4, 0.5); !near(again[11], got[11]) {
		t.Fatalf("a later value changed the request at its own step: %v, was %v", again[11], got[11])
	}
	capped := ShapeLearned([]float64{9, 9, 9, 9}, flat(5, 4), flat(0, 4), 1, 2, 0)
	if capped[3] != 5 {
		t.Fatalf("request above the template: %v", capped)
	}
}

// VPA learned from a trailing window, updated every few steps, may rise above
// the template.
func TestVPALearned(t *testing.T) {
	usage := []float64{1, 1, 1, 1, 8, 8, 8, 8}
	got := VPALearned(usage, flat(4, 8), 2, 4, 1, 0)
	want := []float64{4, 4, 1, 1, 1, 1, 8, 8}
	for i := range want {
		if !near(got[i], want[i]) {
			t.Fatalf("step %d: %v, want %v (all %v)", i, got[i], want[i], got)
		}
	}
}

// Memory usage beyond a node's memory is counted, as CPU usage beyond its
// CPUs is: requests below usage allow it.
func TestMemoryOverload(t *testing.T) {
	nodes := withMem([]NodeSpec{node("a", 10, 100, 300, time.Minute)})
	cfg := Config{Step: time.Minute, Nodes: nodes, Demand: flat(5, 4), MemDemand: flat(8, 4),
		MemUsage: []float64{8, 11, 9, 12}, UseRatio: 1}
	r, err := Run(cfg, AlwaysOn{})
	if err != nil {
		t.Fatal(err)
	}
	if r.MemOverloadMinutes != 2 {
		t.Fatalf("MemOverloadMinutes = %v, want 2", r.MemOverloadMinutes)
	}
}
