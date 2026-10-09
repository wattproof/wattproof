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

	"github.com/wattproof/wattproof/internal/sim"
)

// Cell a's capacity in the trace's normalised units (analysis/traces/
// google2019_capacity.sql). Memory requests are scaled with the CPU requests
// so that the cell's balance between them carries over to the simulated
// workers, whose memory equals their CPUs.
const (
	cellCPU   = 6835.69
	cellMem   = 4103.95
	traceFile = "google2019-cella-services.csv"
	hourSteps = 60
	trainDays = 7
)

// traceGroup is one group's hourly series, by column name.
type traceGroup map[string][]float64

func readTrace(path string) (map[string]traceGroup, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	groups := map[string]traceGroup{}
	var order, header []string
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
		g := fields[1]
		if groups[g] == nil {
			groups[g] = traceGroup{}
			order = append(order, g)
		}
		for i := 2; i < len(fields); i++ {
			v, err := strconv.ParseFloat(fields[i], 64)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %q: %w", path, line, err)
			}
			groups[g][header[i]] = append(groups[g][header[i]], v)
		}
	}
	return groups, order, sc.Err()
}

// minutes interpolates hourly values to one per minute and scales them.
func minutes(hourly []float64, scale float64) []float64 {
	out := sim.Interpolate(hourly, hourSteps)
	for i := range out {
		out[i] *= scale
	}
	return out
}

// runTrace runs the arms on the Google 2019 trace's services: four weeks
// scored after a week of learning, memory as a second limit, and the servers'
// idle power varied, the number published curves transfer worst.
func runTrace(dir string) error {
	groups, order, err := readTrace(filepath.Join(dir, traceFile))
	if err != nil {
		return err
	}
	r7725, err := load(dir, "specpower-dell-r7725-2024.csv", "R7725")
	if err != nil {
		return err
	}
	r7425, err := load(dir, "specpower-dell-r7425-2018.csv", "R7425")
	if err != nil {
		return err
	}
	fmt.Println("TRACE: Google cluster trace 2019, cell a, long-running services (boot 5 min)")
	fmt.Printf("Requests scaled so the template's peak is %.0f%% of worker CPU; memory keeps the cell's\n", peakShare*100)
	fmt.Println("balance with CPU, and each worker's memory equals its CPUs. Week 1 trains; weeks 2-5 are")
	fmt.Println("scored. R: CPU shaped from earlier weeks (hourly peak +20%). M: memory set to the earlier")
	fmt.Println("weeks' peak +15%. VPA: CPU and memory at p90 of the last 8 days +15%, updated daily.")
	fmt.Println("idle: the servers' idle power as a share of peak (published: 16% R7725, 30% R7425).")
	fmt.Println()
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "services\tworkers\tidle\tB0 kWh\tB1/B0\tB1+VPA/B0\tT/B0\tT+R/B0\tT+RM/B0\tT+R/(B1+VPA)\tT+RM/(B1+VPA)\tT+R/T\tsame, no memory\tpending min B1+VPA\tT+R\tT+RM\tmemory over min B1+VPA\tT+R\tT+RM\t")
	for _, g := range order {
		tg := groups[g]
		for _, hw := range []string{"6 new", "3 new+3 old"} {
			for _, idle := range []float64{0, 0.3, 0.5} {
				servers := []server{r7725, r7725, r7725, r7725, r7725, r7725}
				if hw != "6 new" {
					servers = []server{r7725, r7425, r7725, r7425, r7725, r7425}
				}
				label := "published"
				if idle > 0 {
					label = fmt.Sprintf("%.0f%%", idle*100)
					servers = slices.Clone(servers)
					for i := range servers {
						c, err := servers[i].curve.WithIdleShare(idle)
						if err != nil {
							return err
						}
						servers[i].curve = c
					}
				}
				var capacity float64
				for _, s := range servers {
					capacity += s.cpus
				}
				f := peakShare * capacity / slices.Max(tg["cpu_req"])
				fm := f * cellCPU / cellMem
				usage := minutes(tg["cpu_use"], f)
				req, shaped, vpa := minutes(tg["cpu_req"], f), minutes(tg["cpu_shaped"], f), minutes(tg["cpu_vpa"], f)
				mem, memVPA, memPeak := minutes(tg["mem_req"], fm), minutes(tg["mem_vpa"], fm), minutes(tg["mem_peak"], fm)
				memUse := minutes(tg["mem_use"], fm)
				planner := func(label string) *sim.Planner {
					p := sim.NewPlanner(sim.SeasonalNaive{Period: 24 * hourSteps})
					p.MemForecast = sim.SeasonalNaive{Period: 24 * hourSteps}
					p.Label = label
					return p
				}
				res, err := runArmsFrom(servers, 5*time.Minute, usage, r7725.curve.Idle(), []arm{
					{sim.AlwaysOn{}, req, mem, memUse},
					{sim.NewAutoscaler(), req, mem, memUse},
					{sim.NewAutoscaler(), vpa, memVPA, memUse},
					{planner("T"), req, mem, memUse},
					{planner("T+R"), shaped, mem, memUse},
					{planner("T+RM"), shaped, memPeak, memUse},
					{planner("T+R, no memory"), shaped, nil, nil},
					{planner("T, no memory"), req, nil, nil},
				}, trainDays*24*hourSteps)
				if err != nil {
					return fmt.Errorf("%s, %s, idle %s: %w", g, hw, label, err)
				}
				b0, b1, bv, t1, tr, trm, trN, tN := res[0], res[1], res[2], res[3], res[4], res[5], res[6], res[7]
				fmt.Fprintf(tw, "%s\t%s\t%s\t%.0f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.3f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t%.0f\t\n",
					g, hw, label, b0.KWh(), b1.Joules/b0.Joules, bv.Joules/b0.Joules, t1.Joules/b0.Joules,
					tr.Joules/b0.Joules, trm.Joules/b0.Joules, tr.Joules/bv.Joules, trm.Joules/bv.Joules,
					tr.Joules/t1.Joules, trN.Joules/tN.Joules,
					bv.ShortfallMinutes, tr.ShortfallMinutes, trm.ShortfallMinutes,
					bv.MemOverloadMinutes, tr.MemOverloadMinutes, trm.MemOverloadMinutes)
				if t := bv.OverloadMinutes + tr.OverloadMinutes + trm.OverloadMinutes; t > 0 {
					return fmt.Errorf("%s, %s: CPU overload of %v minutes; add a column for it", g, hw, t)
				}
			}
		}
	}
	tw.Flush()
	fmt.Println()
	fmt.Println("same, no memory: T+R/T with memory ignored, as before the trace analysis plan. The gap to")
	fmt.Println("T+R/T is what ignoring memory overstated. memory over min: minutes in which some node's")
	fmt.Println("memory usage exceeded its memory (evictions or pods killed). No arm's CPU usage exceeded capacity.")
	return nil
}
