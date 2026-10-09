// Command wattproof-sim runs the cluster simulator over a set of scenarios and
// prints the predicted energy of each arm. The output is a simulation: a
// prior for experiment design, never a measured saving (ADR-0011).
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/wattproof/wattproof/internal/power"
	"github.com/wattproof/wattproof/internal/sim"
)

// Assumptions not taken from a measurement. Each is printed with the results.
const (
	peakShare = 0.7  // peak requested CPU as a share of worker capacity
	useRatio  = 0.5  // CPU usage divided by CPU requests, at peak; swept below
	offWatts  = 10.0 // standby power of a switched-off server
	bootLoad  = 0.3  // a booting server draws its power at this load
	minNodes  = 1

	// Request shaping (ADR-0010) and the Vertical Pod Autoscaler's defaults.
	resizeMargin = 0.2  // shaped request = hourly peak usage + 20%
	resizeFloor  = 0.1  // never below 10% of the template's request
	vpaQuantile  = 0.9  // VPA recommends p90 of usage ...
	vpaMargin    = 0.15 // ... plus 15%
)

func main() {
	dir := flag.String("testdata", "testdata", "directory with the curves and the demand shape")
	only := flag.String("only", "", `run one part only: "trace" for the trace experiment, "gpu" for GPU inference`)
	flag.Parse()
	var err error
	switch *only {
	case "trace":
		err = runTrace(*dir)
	case "gpu":
		err = runGPU(*dir)
	case "":
		err = run(*dir)
	default:
		err = fmt.Errorf("unknown part %q", *only)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wattproof-sim:", err)
		os.Exit(1)
	}
}

type server struct {
	name  string
	curve power.Curve
	cpus  float64
}

func load(dir, file, name string) (server, error) {
	f, err := os.Open(filepath.Join(dir, file))
	if err != nil {
		return server{}, err
	}
	defer f.Close()
	c, cpus, err := power.ReadCurve(f)
	if err != nil {
		return server{}, fmt.Errorf("%s: %w", file, err)
	}
	return server{name: name, curve: c, cpus: float64(cpus)}, nil
}

func (s server) spec(boot time.Duration) sim.NodeSpec {
	return sim.NodeSpec{Name: s.name, CPUs: s.cpus, Power: power.Node{
		Curve: s.curve, OffWatts: offWatts,
		Transition: power.Transition{
			Shutdown: time.Minute, ShutdownJoules: 60 * s.curve.Idle(),
			Boot: boot, BootJoules: boot.Seconds() * s.curve.Watts(bootLoad),
		},
	}}
}

type hardware struct {
	name    string
	servers []server
}

func run(dir string) error {
	r7725, err := load(dir, "specpower-dell-r7725-2024.csv", "R7725")
	if err != nil {
		return err
	}
	r7425, err := load(dir, "specpower-dell-r7425-2018.csv", "R7425")
	if err != nil {
		return err
	}
	f, err := os.Open(filepath.Join(dir, "demand-dewiki-2025-09-week.csv"))
	if err != nil {
		return err
	}
	shape, err := sim.ReadShape(f)
	f.Close()
	if err != nil {
		return err
	}
	week := sim.Interpolate(shape, 60) // one value per minute
	hws := []hardware{
		{"6 new", []server{r7725, r7725, r7725, r7725, r7725, r7725}},
		{"3 new+3 old", []server{r7725, r7425, r7725, r7425, r7725, r7425}},
	}
	controlPlane := r7725.curve.Idle()

	fmt.Println("SIMULATION, not a measurement (ADR-0011).")
	fmt.Printf("Assumed: peak requests %.0f%% of worker capacity, usage %.0f%% of requests at peak and\n",
		peakShare*100, useRatio*100)
	fmt.Printf("following traffic, %v W off, boot at the power of %.0f%% load, shutdown 1 min at idle,\n",
		offWatts, bootLoad*100)
	fmt.Printf("at least %d worker on. Control plane: one R7725 at idle in every arm. The week runs twice;\n", minNodes)
	fmt.Println("the second is scored. T forecasts from yesterday; T* has a perfect forecast (an upper bound);")
	fmt.Println("Tp is T with MostAllocated scoring, a scheduler change outside v0.1 (ADR-0005).")
	fmt.Println()
	fmt.Println("EXPERIMENT 1: node power state")

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "workers\tfixed\tboot\tB0 kWh\tB1/B0\tT/B0\tT*/B0\tTp/B0\tT/B1\tTp/B1\tpending min B1\tT\tTp\toffs/day B1\tT\t")
	days := float64(len(week)) / (24 * 60)
	for _, hw := range hws {
		for _, fixed := range []float64{0, 0.5} {
			for _, boot := range []time.Duration{2 * time.Minute, 5 * time.Minute, 10 * time.Minute} {
				w := newWorkload(hw.servers, week, fixed, useRatio)
				t := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
				tStar := sim.NewPlanner(sim.Oracle{Series: w.requests})
				tStar.Label = "T*"
				tp := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
				tp.Label, tp.Scheduler = "Tp", sim.Pack
				res, err := runArms(hw.servers, boot, w.usage, controlPlane, []arm{
					{sim.AlwaysOn{}, w.requests, nil, nil}, {sim.NewAutoscaler(), w.requests, nil, nil},
					{t, w.requests, nil, nil}, {tStar, w.requests, nil, nil}, {tp, w.requests, nil, nil},
				})
				if err != nil {
					return err
				}
				b0, b1, t1, ts, tpr := res[0], res[1], res[2], res[3], res[4]
				fmt.Fprintf(tw, "%s\t%.0f%%\t%v\t%.0f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.0f\t%.0f\t%.0f\t%.1f\t%.1f\t\n",
					hw.name, fixed*100, boot, b0.KWh(),
					b1.Joules/b0.Joules, t1.Joules/b0.Joules, ts.Joules/b0.Joules, tpr.Joules/b0.Joules,
					t1.Joules/b1.Joules, tpr.Joules/b1.Joules,
					b1.ShortfallMinutes, t1.ShortfallMinutes, tpr.ShortfallMinutes,
					float64(b1.PowerOffs)/days, float64(t1.PowerOffs)/days)
			}
		}
	}
	tw.Flush()
	fmt.Println()
	fmt.Println("fixed: share of peak requests from services with fixed replicas. Their usage follows")
	fmt.Println("traffic; their requests do not. pending min: minutes with requests above Ready capacity.")
	fmt.Println()

	fmt.Println("EXPERIMENT 2: diurnal CPU rightsizing (boot 5 min)")
	fmt.Printf("R: requests of fixed-replica services follow hourly peak usage +%.0f%%, floor %.0f%% of the\n",
		resizeMargin*100, resizeFloor*100)
	fmt.Printf("template. VPA: one constant request, p%.0f of usage +%.0f%%. Autoscaled services unchanged.\n",
		vpaQuantile*100, vpaMargin*100)
	fmt.Println()
	tw = tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "workers\tfixed\tB0 kWh\tB1/B0\tB1+VPA/B0\tT/B0\tT+R/B0\tT+R/T\tT+R/(B1+VPA)\tpending min B1+VPA\tT+R\tthrottled min B1+VPA\tT+R\t")
	for _, hw := range hws {
		for _, fixed := range []float64{0.5, 0.8} {
			w := newWorkload(hw.servers, week, fixed, useRatio)
			t := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
			tr := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
			tr.Label = "T+R"
			b1vpa := sim.NewAutoscaler()
			res, err := runArms(hw.servers, 5*time.Minute, w.usage, controlPlane, []arm{
				{sim.AlwaysOn{}, w.requests, nil, nil}, {sim.NewAutoscaler(), w.requests, nil, nil},
				{b1vpa, w.vpa(), nil, nil}, {t, w.requests, nil, nil}, {tr, w.shaped(), nil, nil},
			})
			if err != nil {
				return err
			}
			b0, b1, bv, t1, trr := res[0], res[1], res[2], res[3], res[4]
			fmt.Fprintf(tw, "%s\t%.0f%%\t%.0f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.0f\t%.0f\t%.0f\t%.0f\t\n",
				hw.name, fixed*100, b0.KWh(),
				b1.Joules/b0.Joules, bv.Joules/b0.Joules, t1.Joules/b0.Joules, trr.Joules/b0.Joules,
				trr.Joules/t1.Joules, trr.Joules/bv.Joules,
				bv.ShortfallMinutes, trr.ShortfallMinutes, bv.OverloadMinutes, trr.OverloadMinutes)
		}
	}
	tw.Flush()
	fmt.Println()
	fmt.Println("throttled min: minutes in which some node's CPU usage exceeded its capacity.")
	fmt.Println()
	return sweep(dir, hws, week, controlPlane)
}

// sweep repeats both experiments across usage-to-request ratios, the
// assumption the results depend on most.
func sweep(dir string, hws []hardware, week []float64, controlPlane float64) error {
	fmt.Println("SENSITIVITY: usage as a share of requests at peak (boot 5 min)")
	fmt.Println("Every column uses the row's share of fixed-replica requests.")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "workers	usage/req	fixed	B1/B0	T/B1	B1+VPA/B0	T+R/B0	T+R/T	T+R/(B1+VPA)	pending min B1+VPA	T+R	throttled B1+VPA	T+R\t")
	for _, hw := range hws {
		for _, ratio := range []float64{0.2, 0.35, 0.5, 0.7, 0.9} {
			for _, fixed := range []float64{0.5, 0.8} {
				w := newWorkload(hw.servers, week, fixed, ratio)
				tr := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
				tr.Label = "T+R"
				res, err := runArms(hw.servers, 5*time.Minute, w.usage, controlPlane, []arm{
					{sim.AlwaysOn{}, w.requests, nil, nil}, {sim.NewAutoscaler(), w.requests, nil, nil},
					{sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60}), w.requests, nil, nil},
					{sim.NewAutoscaler(), w.vpa(), nil, nil}, {tr, w.shaped(), nil, nil},
				})
				if err != nil {
					return err
				}
				b0, b1, t1, bv, trr := res[0], res[1], res[2], res[3], res[4]
				fmt.Fprintf(tw, "%s\t%.0f%%\t%.0f%%\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.0f\t%.0f\t%.0f\t%.0f\t\n",
					hw.name, ratio*100, fixed*100, b1.Joules/b0.Joules, t1.Joules/b1.Joules,
					bv.Joules/b0.Joules, trr.Joules/b0.Joules, trr.Joules/t1.Joules, trr.Joules/bv.Joules,
					bv.ShortfallMinutes, trr.ShortfallMinutes, bv.OverloadMinutes, trr.OverloadMinutes)
			}
		}
	}
	tw.Flush()
	fmt.Println()
	return runTrace(dir)
}

// workload is two weeks of requests and usage: autoscaled services, whose
// requests follow traffic, and fixed-replica services, whose requests do not.
type workload struct {
	scaledReq, fixedUse, usage, requests []float64
	fixedReq                             float64
}

func newWorkload(servers []server, week []float64, fixed, useRatio float64) workload {
	var capacity float64
	for _, s := range servers {
		capacity += s.cpus
	}
	w := workload{fixedReq: peakShare * capacity * fixed}
	for range 2 {
		for _, s := range week {
			scaled := peakShare * capacity * (1 - fixed) * s
			fixedUse := w.fixedReq * useRatio * s
			w.scaledReq = append(w.scaledReq, scaled)
			w.fixedUse = append(w.fixedUse, fixedUse)
			w.usage = append(w.usage, scaled*useRatio+fixedUse)
			w.requests = append(w.requests, scaled+w.fixedReq)
		}
	}
	return w
}

// shaped returns requests with the fixed-replica services resized by hour.
func (w workload) shaped() []float64 {
	r := sim.ShapeRequests(w.fixedUse, 60, w.fixedReq, resizeMargin, resizeFloor*w.fixedReq)
	for i := range r {
		r[i] += w.scaledReq[i]
	}
	return r
}

// vpa returns requests with the fixed-replica services set to VPA's
// constant recommendation, learned over the first week.
func (w workload) vpa() []float64 {
	target := sim.VPATarget(w.fixedUse[:len(w.fixedUse)/2], vpaQuantile, vpaMargin)
	r := make([]float64, len(w.scaledReq))
	for i := range r {
		r[i] = w.scaledReq[i] + target
	}
	return r
}

type arm struct {
	policy   sim.Policy
	requests []float64
	// mem is the requested memory, or nil when memory is not modelled.
	mem []float64
	// memUse is the memory in use, or nil.
	memUse []float64
}

// runArms runs each arm on the same servers and usage, the second week scored.
func runArms(servers []server, boot time.Duration, usage []float64, controlPlane float64, arms []arm) ([]sim.Result, error) {
	return runArmsFrom(servers, boot, usage, controlPlane, arms, len(usage)/2)
}

// runArmsFrom runs each arm on the same servers and usage, scored from step
// score on. Each server's memory equals its CPUs, in the unit of the arms'
// memory series.
func runArmsFrom(servers []server, boot time.Duration, usage []float64, controlPlane float64, arms []arm, score int) ([]sim.Result, error) {
	var nodes []sim.NodeSpec
	for _, s := range servers {
		n := s.spec(boot)
		n.Mem = s.cpus
		nodes = append(nodes, n)
	}
	var out []sim.Result
	for _, a := range arms {
		cfg := sim.Config{Step: time.Minute, Nodes: nodes, Demand: a.requests, Usage: usage, MemDemand: a.mem, MemUsage: a.memUse,
			MinNodes: minNodes, BaseWatts: controlPlane, Score: score}
		r, err := sim.Run(cfg, a.policy)
		if err != nil {
			return nil, err
		}
		if r.Rejected != 0 {
			return nil, fmt.Errorf("%s: %d power-offs rejected by the invariants", r.Policy, r.Rejected)
		}
		out = append(out, r)
	}
	return out, nil
}
