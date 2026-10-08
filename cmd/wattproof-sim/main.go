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
	useRatio  = 0.5  // CPU usage divided by CPU requests
	offWatts  = 10.0 // standby power of a switched-off server
	bootLoad  = 0.3  // a booting server draws its power at this load
	minNodes  = 1
)

func main() {
	dir := flag.String("testdata", "testdata", "directory with the curves and the demand shape")
	flag.Parse()
	if err := run(*dir); err != nil {
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

	fmt.Println("SIMULATION, not a measurement (ADR-0011).")
	fmt.Printf("Assumed: peak requests %.0f%% of worker capacity, usage %.0f%% of requests, %v W off,\n",
		peakShare*100, useRatio*100, offWatts)
	fmt.Printf("boot at the power of %.0f%% load, shutdown 1 min at idle, at least %d worker on.\n",
		bootLoad*100, minNodes)
	fmt.Println("Control plane: one R7725 at idle, in every arm. Week replayed twice; the second is scored.")
	fmt.Println("T forecasts from yesterday (seasonal naive); T* has a perfect forecast (an upper bound);")
	fmt.Println("Tp is T with MostAllocated scoring, a scheduler change outside v0.1 (ADR-0005).")
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "workers\tfixed\tboot\tB0 kWh\tB1/B0\tT/B0\tT*/B0\tTp/B0\tT/B1\tTp/B1\tpending min B1\tT\tTp\toffs/day B1\tT\t")
	for _, hw := range []struct {
		name    string
		servers []server
	}{
		{"6 new", []server{r7725, r7725, r7725, r7725, r7725, r7725}},
		{"3 new+3 old", []server{r7725, r7425, r7725, r7425, r7725, r7425}},
	} {
		for _, fixed := range []float64{0, 0.5} {
			for _, boot := range []time.Duration{2 * time.Minute, 5 * time.Minute, 10 * time.Minute} {
				res, err := scenario(hw.servers, week, fixed, boot, r7725.curve.Idle())
				if err != nil {
					return err
				}
				b0, b1, t, tStar, tp := res[0], res[1], res[2], res[3], res[4]
				days := float64(len(week)) / (24 * 60)
				fmt.Fprintf(tw, "%s\t%.0f%%\t%v\t%.0f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.0f\t%.0f\t%.0f\t%.1f\t%.1f\t\n",
					hw.name, fixed*100, boot, b0.KWh(),
					b1.Joules/b0.Joules, t.Joules/b0.Joules, tStar.Joules/b0.Joules, tp.Joules/b0.Joules,
					t.Joules/b1.Joules, tp.Joules/b1.Joules,
					b1.ShortfallMinutes, t.ShortfallMinutes, tp.ShortfallMinutes,
					float64(b1.PowerOffs)/days, float64(t.PowerOffs)/days)
			}
		}
	}
	tw.Flush()
	fmt.Println()
	fmt.Println("fixed: share of peak requests that never scales down (fixed replicas).")
	fmt.Println("pending min: minutes in the scored week with requests above Ready capacity.")
	return nil
}

// scenario runs B0, B1, T, T* and Tp and returns their results in that order.
func scenario(servers []server, week []float64, fixed float64, boot time.Duration, controlPlane float64) ([]sim.Result, error) {
	var nodes []sim.NodeSpec
	var capacity float64
	for _, s := range servers {
		nodes = append(nodes, s.spec(boot))
		capacity += s.cpus
	}
	demand := make([]float64, 0, 2*len(week))
	for range 2 {
		for _, s := range week {
			demand = append(demand, peakShare*capacity*(fixed+(1-fixed)*s))
		}
	}
	cfg := sim.Config{Step: time.Minute, Nodes: nodes, Demand: demand, UseRatio: useRatio,
		MinNodes: minNodes, BaseWatts: controlPlane, Score: len(week)}
	tStar := sim.NewPlanner(sim.Oracle{Series: demand})
	tStar.Label = "T*"
	tp := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
	tp.Label, tp.Scheduler = "Tp", sim.Pack
	var out []sim.Result
	for _, p := range []sim.Policy{sim.AlwaysOn{}, sim.NewAutoscaler(),
		sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60}), tStar, tp} {
		r, err := sim.Run(cfg, p)
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
