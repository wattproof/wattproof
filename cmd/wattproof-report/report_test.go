package main

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wattproof/wattproof/internal/sim"
)

const testHours = 15 * day

// synthetic writes a cluster with known requests and usage:
//   - web: busy 09:00-17:00 (highest 5 minutes 1.0, else 0.25), request 4;
//     memory 2, with 3 at hour 100 (before the last 8 days) and 2.5 at hour
//     300 (inside them), request 8;
//   - db: excluded (Guaranteed QoS), request 2 CPU, 4 memory;
//   - batch: only the last 100 hours, request 1 and 1;
//   - old: only the first 101 hours.
func synthetic() string {
	var b strings.Builder
	b.WriteString("workload,hour,cpu_use,cpu_peak,cpu_req,mem_use,mem_req,exclude\n")
	for h := range testHours {
		peak := 0.25
		if hd := h % day; hd >= 9 && hd < 17 {
			peak = 1
		}
		mem := 2.0
		switch h {
		case 100:
			mem = 3
		case 300:
			mem = 2.5
		}
		fmt.Fprintf(&b, "web,%d,%g,%g,4,%g,8,\n", h, 0.8*peak, peak, mem)
		fmt.Fprintf(&b, "db,%d,1,1.2,2,3,4,Guaranteed QoS\n", h)
		if h >= testHours-100 {
			fmt.Fprintf(&b, "batch,%d,0.5,0.6,1,0.5,1,\n", h)
		}
		if h <= 100 {
			fmt.Fprintf(&b, "old,%d,0.5,0.6,1,0.5,1,\n", h)
		}
	}
	return b.String()
}

const nodeFile = `node,cpus,mem,curve,local_storage_pod
n1,5,10,specpower-dell-r7725-2024.csv,
n2,5,10,specpower-dell-r7725-2024.csv,
n3,5,10,specpower-dell-r7425-2018.csv,
`

func buildSynthetic(t *testing.T, nodes string) *report {
	t.Helper()
	ws, hours, err := readWorkloads(strings.NewReader(synthetic()))
	if err != nil {
		t.Fatal(err)
	}
	ns, err := readNodes(strings.NewReader(nodes))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := build(ws, hours, ns, "../../testdata", 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The recommended requests and the freed CPU and memory match hand-computed
// values.
func TestRecommendationsByHand(t *testing.T) {
	rep := buildSynthetic(t, nodeFile)
	web := rep.recs["web"]
	if web == nil {
		t.Fatalf("no recommendation for web: %v", rep.excluded)
	}
	for p := range week {
		want := 0.4 // 0.25 × 1.2 = 0.3, below the floor of 10% of 4
		if hd := p % day; hd >= 9 && hd < 17 {
			want = 1.2
		}
		if !near(web.cpu[p], want) {
			t.Errorf("web CPU at hour %d = %v, want %v", p, web.cpu[p], want)
		}
	}
	if !near(web.mem, 2.5*1.15) {
		t.Errorf("web memory = %v, want %v (the spike before the last 8 days is not counted)", web.mem, 2.5*1.15)
	}
	if rep.peak != 9 {
		t.Errorf("peak hour = %d, want 9", rep.peak)
	}
	// At the peak: web 4 -> 1.2; db and batch keep 2 and 1; old is not running.
	if !near(rep.curCPU, 7) || !near(rep.recCPU, 4.2) {
		t.Errorf("CPU at peak %v -> %v, want 7 -> 4.2", rep.curCPU, rep.recCPU)
	}
	if !near(rep.curMem, 13) || !near(rep.recMem, 2.875+4+1) {
		t.Errorf("memory %v -> %v, want 13 -> 7.875", rep.curMem, rep.recMem)
	}
	if rep.nodesCur != 2 || rep.nodesRec != 1 {
		t.Errorf("nodes needed %d -> %d, want 2 -> 1", rep.nodesCur, rep.nodesRec)
	}
}

// Workloads that must not be rightsized get no recommendation, and each has
// its reason.
func TestExcludedHaveReasons(t *testing.T) {
	rep := buildSynthetic(t, nodeFile)
	want := map[string]string{
		"db":    "Guaranteed QoS",
		"batch": "short history: 100 hours, need 192",
		"old":   "not running in the last 8 days",
	}
	for name, why := range want {
		if rep.recs[name] != nil {
			t.Errorf("%s has a recommendation", name)
		}
		if rep.excluded[name] != why {
			t.Errorf("%s: reason %q, want %q", name, rep.excluded[name], why)
		}
	}
	var csv bytes.Buffer
	if err := rep.writeRecommendations(&csv); err != nil {
		t.Fatal(err)
	}
	for name := range want {
		if strings.Contains(csv.String(), "\n"+name+",") {
			t.Errorf("%s is in the recommendations file", name)
		}
	}
	if n := strings.Count(csv.String(), "\nweb,"); n != week {
		t.Errorf("web has %d rows in the recommendations file, want %d", n, week)
	}
}

// The report's energy for B0 and T equals a direct simulator run on the same
// inputs, assembled here without the report's code.
func TestEnergyMatchesDirectRun(t *testing.T) {
	rep := buildSynthetic(t, nodeFile)
	if rep.energy == nil {
		t.Fatalf("no energy estimate: %s", rep.energyNote)
	}
	cpu, mem := make([]float64, testHours), make([]float64, testHours)
	use, memUse := make([]float64, testHours), make([]float64, testHours)
	for h := range testHours {
		peak := 0.25
		if hd := h % day; hd >= 9 && hd < 17 {
			peak = 1
		}
		m := 2.0
		switch h {
		case 100:
			m = 3
		case 300:
			m = 2.5
		}
		cpu[h], mem[h], use[h], memUse[h] = 4+2, 8+4, 0.8*peak+1, m+3
		if h >= testHours-100 {
			cpu[h], mem[h], use[h], memUse[h] = cpu[h]+1, mem[h]+1, use[h]+0.5, memUse[h]+0.5
		}
		if h <= 100 {
			cpu[h], mem[h], use[h], memUse[h] = cpu[h]+1, mem[h]+1, use[h]+0.5, memUse[h]+0.5
		}
	}
	ns, _ := readNodes(strings.NewReader(nodeFile))
	curves, err := loadCurves(ns, "../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	cfg := sim.Config{Step: time.Minute, Nodes: specs(ns, curves, 5*time.Minute),
		Demand: sim.Interpolate(cpu, 60), Usage: sim.Interpolate(use, 60),
		MemDemand: sim.Interpolate(mem, 60), MemUsage: sim.Interpolate(memUse, 60),
		MinNodes: 1, Score: week * 60}
	b0, err := sim.Run(cfg, sim.AlwaysOn{})
	if err != nil {
		t.Fatal(err)
	}
	p := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
	p.MemForecast = sim.SeasonalNaive{Period: 24 * 60}
	tr, err := sim.Run(cfg, p)
	if err != nil {
		t.Fatal(err)
	}
	if !near(rep.energy.b0.Joules, b0.Joules) || !near(rep.energy.t.Joules, tr.Joules) {
		t.Errorf("report B0 %v J, T %v J; direct %v J, %v J", rep.energy.b0.Joules, rep.energy.t.Joules, b0.Joules, tr.Joules)
	}
	trw := rep.energy.trw
	// Rightsized CPU and memory fit on one node at night, where current
	// requests need two: T+RW keeps fewer node-hours powered than T.
	if trw.NodeHours >= tr.NodeHours {
		t.Errorf("T+RW kept %v node-hours powered, T %v", trw.NodeHours, tr.NodeHours)
	}
	if trw.Joules >= b0.Joules || trw.ShortfallMinutes != 0 || trw.MemOverloadMinutes != 0 {
		t.Errorf("T+RW: %v J against B0 %v J, %v pending and %v memory-over minutes",
			trw.Joules, b0.Joules, trw.ShortfallMinutes, trw.MemOverloadMinutes)
	}
}

// A node on node-local storage is listed with its pod, and energy is not
// estimated until the simulator can hold a node on.
func TestLocalStorageKeepsNodeOn(t *testing.T) {
	rep := buildSynthetic(t, strings.Replace(nodeFile, "2018.csv,", "2018.csv,postgres-0", 1))
	if rep.energy != nil || !strings.Contains(rep.energyNote, "node-local storage") {
		t.Errorf("energy estimated with a node held on: %q", rep.energyNote)
	}
	var out bytes.Buffer
	rep.write(&out)
	if !strings.Contains(out.String(), "n3: postgres-0") {
		t.Errorf("report does not list n3 with its pod:\n%s", out.String())
	}
}

// Nodes are counted most efficient first, until both CPU and memory fit.
func TestNodesNeeded(t *testing.T) {
	ns, _ := readNodes(strings.NewReader(nodeFile))
	curves, err := loadCurves(ns, "../../testdata")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		cpu, mem float64
		want     int
	}{{0, 0, 0}, {5, 1, 1}, {5.1, 1, 2}, {1, 10.1, 2}, {15, 30, 3}, {15.1, 1, -1}, {1, 30.1, -1}} {
		if got := nodesNeeded(ns, curves, c.cpu, c.mem); got != c.want {
			t.Errorf("nodesNeeded(%v CPU, %v memory) = %d, want %d", c.cpu, c.mem, got, c.want)
		}
	}
	// The same curve on twice the CPUs is more efficient per CPU, so the
	// larger node is counted first and holds 5 CPUs alone.
	mixed, _ := readNodes(strings.NewReader("node,cpus,mem,curve\nsmall,4,8,specpower-dell-r7725-2024.csv\nlarge,8,16,specpower-dell-r7725-2024.csv\n"))
	if got := nodesNeeded(mixed, curves, 5, 1); got != 1 {
		t.Errorf("mixed nodes: %d needed for 5 CPUs, want 1 (the larger node first)", got)
	}
}

func TestReadWorkloadsRejectsBadInput(t *testing.T) {
	for name, in := range map[string]string{
		"missing column": "workload,hour,cpu_use\nweb,0,1\n",
		"duplicate hour": "workload,hour,cpu_use,cpu_peak,cpu_req,mem_use,mem_req\nweb,0,1,1,1,1,1\nweb,0,1,1,1,1,1\n",
		"negative usage": "workload,hour,cpu_use,cpu_peak,cpu_req,mem_use,mem_req\nweb,0,-1,1,1,1,1\n",
		"bad hour":       "workload,hour,cpu_use,cpu_peak,cpu_req,mem_use,mem_req\nweb,x,1,1,1,1,1\n",
	} {
		if _, _, err := readWorkloads(strings.NewReader(in)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}
