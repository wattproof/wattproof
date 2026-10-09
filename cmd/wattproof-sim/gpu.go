package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/wattproof/wattproof/internal/power"
	"github.com/wattproof/wattproof/internal/sim"
)

// GPU inference scenario (ADR-0014, ROADMAP Phase 1G). Nobody has published
// the wall power of an idle or powered-off GPU node, so the idle share and the
// model load time are swept instead of assumed. Each node is one model replica
// slot: its capacity is 1, and requests are replicas.
const (
	gpuFile       = "azure2024-llm-minutes.csv"
	gpuNodes      = 20
	gpuPeakShare  = 0.9    // peak replicas as a share of the nodes: sized for the week's peak
	gpuTargetUtil = 0.7    // replica autoscaler target: throughput used per replica
	gpuPeakWatts  = 6000.0 // nominal node power at full throughput; ratios do not depend on it
	gpuOffWatts   = 50.0   // BMC and standby of large power supplies
	gpuServerBoot = 5 * time.Minute
	gpuMinNodes   = 2
	gpuCtxWeight  = 0.1 // a context (prefill) token costs a tenth of a generated token
	gpuHysteresis = time.Hour
)

// readGPUTrace returns each service's load per minute: generated tokens plus
// weighted context tokens.
func readGPUTrace(path string) (map[string][]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	load := map[string][]float64{}
	var header []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ",")
		if header == nil {
			header = fields
			continue
		}
		v := map[string]float64{}
		for i := 1; i < len(fields); i++ {
			x, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return nil, fmt.Errorf("%s: %q: %w", path, line, err)
			}
			v[header[i]] = x
		}
		for _, s := range []string{"code", "conv"} {
			load[s] = append(load[s], v[s+"_generated_tokens"]+gpuCtxWeight*v[s+"_context_tokens"])
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	for s, v := range load {
		if len(v) != 7*24*60 || slices.Max(v) == 0 {
			return nil, fmt.Errorf("%s: service %s has %d minutes, want one week", path, s, len(v))
		}
	}
	return load, nil
}

// trailingMean averages each minute with the w-1 minutes before it.
func trailingMean(v []float64, w int) []float64 {
	out := make([]float64, len(v))
	var sum float64
	for i, x := range v {
		sum += x
		if i >= w {
			sum -= v[i-w]
		}
		out[i] = sum / float64(min(i+1, w))
	}
	return out
}

// gpuNode is a GPU node with a linear power curve from idle to peak, whose
// boot includes loading the model. Shutdown takes 2 minutes at idle; boot and
// model load draw the power of 30% load, as booting servers do elsewhere in
// this simulator.
func gpuNode(idleShare float64, modelLoad time.Duration) (sim.NodeSpec, error) {
	c, err := power.NewCurve([]power.Point{{Load: 0, Watts: idleShare * gpuPeakWatts}, {Load: 1, Watts: gpuPeakWatts}})
	if err != nil {
		return sim.NodeSpec{}, err
	}
	boot := gpuServerBoot + modelLoad
	return sim.NodeSpec{Name: "gpu", CPUs: 1, Power: power.Node{
		Curve: c, OffWatts: gpuOffWatts,
		Transition: power.Transition{
			Shutdown: 2 * time.Minute, ShutdownJoules: 120 * c.Idle(),
			Boot: boot, BootJoules: boot.Seconds() * c.Watts(bootLoad),
		},
	}}, nil
}

func runGPU(dir string) error {
	load, err := readGPUTrace(filepath.Join(dir, gpuFile))
	if err != nil {
		return err
	}
	fmt.Println("GPU INFERENCE: node power state on the Azure LLM inference trace 2024 (one week each)")
	fmt.Println("SIMULATION, not a measurement (ADR-0011). The GPU node's wall power is unknown, so its idle")
	fmt.Println("share and model load time are swept; Phase 1G measures them.")
	fmt.Printf("Assumed: %d nodes, one model replica each; replicas follow load averaged over a window,\n", gpuNodes)
	fmt.Printf("%.0f%% of the nodes at the week's peak, each replica at %.0f%% of its throughput; load = generated\n",
		gpuPeakShare*100, gpuTargetUtil*100)
	fmt.Printf("tokens + %.1f × context tokens. Linear power from idle to %.0f W; %v W off; server boot %v plus\n",
		gpuCtxWeight, gpuPeakWatts, gpuOffWatts, gpuServerBoot)
	fmt.Printf("model load, at the power of %.0f%% load; shutdown 2 min at idle; at least %d nodes on.\n", bootLoad*100, gpuMinNodes)
	fmt.Println("The week runs twice; the second is scored. T forecasts from yesterday; T* has a perfect forecast;")
	fmt.Printf("Th is T with a %v margin on the break-even cycle, to switch GPU nodes less often.\n", gpuHysteresis)
	fmt.Println()

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "service\twindow\tidle\tload\tB0 MWh\tB1/B0\tT/B0\tTh/B0\tT*/B0\tT/B1\tnodes on B1\tT\tTh\tpending min B1\tT\tTh\tcycles/node/day B1\tT\tTh\t")
	for _, svc := range []string{"code", "conv"} {
		for _, window := range []int{5, 30} {
			if err := gpuRows(tw, svc, window, load[svc]); err != nil {
				return err
			}
		}
	}
	tw.Flush()
	fmt.Println()
	fmt.Println("window: minutes over which the replica autoscaler averages load. idle: idle power as a share")
	fmt.Println("of peak. load: model load time, added to the server's boot. nodes on: average powered nodes.")
	fmt.Println("pending min: minutes with replicas waiting for a Ready node. cycles/node/day: power-offs per")
	fmt.Println("node per day.")
	fmt.Println("Phase 1G rule: GPU power state continues only if T saves at least 5% of B0's energy.")
	return nil
}

func gpuRows(tw *tabwriter.Writer, svc string, window int, load []float64) error {
	{
		smooth := trailingMean(load, window)
		peak := slices.Max(smooth)
		var requests, usage []float64
		for range 2 {
			for _, x := range smooth {
				r := gpuPeakShare * gpuNodes * x / peak
				requests = append(requests, r)
				usage = append(usage, r*gpuTargetUtil)
			}
		}
		for _, idle := range []float64{0.15, 0.30, 0.45} {
			for _, ml := range []time.Duration{2 * time.Minute, 6 * time.Minute, 10 * time.Minute} {
				spec, err := gpuNode(idle, ml)
				if err != nil {
					return err
				}
				nodes := make([]sim.NodeSpec, gpuNodes)
				for i := range nodes {
					nodes[i] = spec
				}
				tStar := sim.NewPlanner(sim.Oracle{Series: requests})
				tStar.Label = "T*"
				th := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60})
				th.Label, th.Margin = "Th", gpuHysteresis
				var res []sim.Result
				for _, p := range []sim.Policy{sim.AlwaysOn{}, sim.NewAutoscaler(),
					sim.NewPlanner(sim.SeasonalNaive{Period: 24 * 60}), tStar, th} {
					r, err := sim.Run(sim.Config{Step: time.Minute, Nodes: nodes, Demand: requests, Usage: usage,
						MinNodes: gpuMinNodes, Score: len(requests) / 2}, p)
					if err != nil {
						return err
					}
					if r.Rejected != 0 {
						return fmt.Errorf("%s: %d power-offs rejected by the invariants", r.Policy, r.Rejected)
					}
					res = append(res, r)
				}
				b0, b1, t, ts, h := res[0], res[1], res[2], res[3], res[4]
				hours := float64(len(requests)/2) / 60
				perNodeDay := func(r sim.Result) float64 { return float64(r.PowerOffs) / 7 / gpuNodes }
				fmt.Fprintf(tw, "%s\t%d\t%.0f%%\t%v\t%.1f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.1f\t%.1f\t%.1f\t%.0f\t%.0f\t%.0f\t%.2f\t%.2f\t%.2f\t\n",
					svc, window, idle*100, ml, b0.KWh()/1000,
					b1.Joules/b0.Joules, t.Joules/b0.Joules, h.Joules/b0.Joules, ts.Joules/b0.Joules, t.Joules/b1.Joules,
					b1.NodeHours/hours, t.NodeHours/hours, h.NodeHours/hours,
					b1.ShortfallMinutes, t.ShortfallMinutes, h.ShortfallMinutes,
					perNodeDay(b1), perNodeDay(t), perNodeDay(h))
			}
		}
	}
	return nil
}
