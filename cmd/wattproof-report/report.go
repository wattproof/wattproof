package main

import (
	"bufio"
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
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

const (
	day     = 24
	week    = 7 * day
	history = 8 * day // the memory window, and the least history a workload needs (ADR-0013)

	cpuMargin = 0.2  // CPU request = learned hourly peak + 20% (ADR-0010) ...
	cpuFloor  = 0.1  // ... never below 10% of the current request
	memMargin = 0.15 // memory request = trailing 8 days' maximum + 15% (ADR-0013)

	// The simulator's settings, as in wattproof-sim.
	hourSteps = 60
	offWatts  = 10.0 // standby power of a switched-off server
	bootLoad  = 0.3  // a booting server draws its power at this load
	minNodes  = 1
)

type sample struct{ cpuUse, cpuPeak, cpuReq, memUse, memReq float64 }

type workload struct {
	name    string
	exclude string
	rows    map[int]sample
}

type node struct {
	name      string
	cpus, mem float64
	curve     string
	localPod  string
}

// recommendation is one workload's requests for the coming week.
type recommendation struct {
	cpu            [week]float64
	mem            float64
	curCPU, curMem float64 // the requests in the workload's last hour
}

type report struct {
	hours     int
	workloads []*workload
	recs      map[string]*recommendation
	excluded  map[string]string // workload → reason
	current   map[string]sample // every workload that ran in the last 8 days: its last hour
	nodes     []node

	peak                           int // hour of the week with the most recommended CPU
	curCPU, recCPU, curMem, recMem float64
	nodesCur, nodesRec             int // -1: does not fit

	energy     *energyResult
	energyNote string
}

type energyResult struct {
	days       float64
	b0, t, trw sim.Result
}

// readWorkloads reads the workload file and returns the workloads in the
// order they first appear, and the number of hours covered.
func readWorkloads(r io.Reader) ([]*workload, int, error) {
	cr := csv.NewReader(r)
	cr.Comment = '#'
	header, err := cr.Read()
	if err != nil {
		return nil, 0, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	need := []string{"workload", "hour", "cpu_use", "cpu_peak", "cpu_req", "mem_use", "mem_req"}
	for _, n := range need {
		if _, ok := col[n]; !ok {
			return nil, 0, fmt.Errorf("missing column %q", n)
		}
	}
	byName := map[string]*workload{}
	var order []*workload
	hours := 0
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		h, err := strconv.Atoi(strings.TrimSpace(rec[col["hour"]]))
		if err != nil || h < 0 {
			return nil, 0, fmt.Errorf("line %d: hour %q", line, rec[col["hour"]])
		}
		v := make([]float64, 5)
		for i, n := range need[2:] {
			v[i], err = strconv.ParseFloat(strings.TrimSpace(rec[col[n]]), 64)
			if err != nil || v[i] < 0 {
				return nil, 0, fmt.Errorf("line %d: %s %q", line, n, rec[col[n]])
			}
		}
		name := strings.TrimSpace(rec[col["workload"]])
		w := byName[name]
		if w == nil {
			w = &workload{name: name, rows: map[int]sample{}}
			byName[name] = w
			order = append(order, w)
		}
		if i, ok := col["exclude"]; ok {
			if x := strings.TrimSpace(rec[i]); x != "" {
				w.exclude = x
			}
		}
		if _, dup := w.rows[h]; dup {
			return nil, 0, fmt.Errorf("line %d: %s has hour %d twice", line, name, h)
		}
		w.rows[h] = sample{v[0], v[1], v[2], v[3], v[4]}
		hours = max(hours, h+1)
	}
	if len(order) == 0 {
		return nil, 0, errors.New("no workloads")
	}
	return order, hours, nil
}

func readNodes(r io.Reader) ([]node, error) {
	cr := csv.NewReader(r)
	cr.Comment = '#'
	cr.FieldsPerRecord = -1
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	for _, n := range []string{"node", "cpus", "mem", "curve"} {
		if _, ok := col[n]; !ok {
			return nil, fmt.Errorf("missing column %q", n)
		}
	}
	var out []node
	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		field := func(n string) string {
			i, ok := col[n]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		cpus, err1 := strconv.ParseFloat(field("cpus"), 64)
		mem, err2 := strconv.ParseFloat(field("mem"), 64)
		if errors.Join(err1, err2) != nil || cpus <= 0 || mem <= 0 || field("curve") == "" {
			return nil, fmt.Errorf("line %d: need a name, positive cpus and mem, and a curve", line)
		}
		out = append(out, node{name: field("node"), cpus: cpus, mem: mem, curve: field("curve"), localPod: field("local_storage_pod")})
	}
	if len(out) == 0 {
		return nil, errors.New("no nodes")
	}
	return out, nil
}

// recommend returns the workload's requests for the coming week, or the
// reason it gets none.
func recommend(w *workload, hours int) (*recommendation, string) {
	if w.exclude != "" {
		return nil, w.exclude
	}
	if len(w.rows) < history {
		return nil, fmt.Sprintf("short history: %d hours, need %d", len(w.rows), history)
	}
	r := &recommendation{}
	last := -1
	var peak [week]float64
	var seen [week]bool
	for h, s := range w.rows {
		p := h % week
		peak[p] = max(peak[p], s.cpuPeak)
		seen[p] = true
		if h >= hours-history {
			r.mem = max(r.mem, s.memUse)
		}
		last = max(last, h)
	}
	r.curCPU, r.curMem = w.rows[last].cpuReq, w.rows[last].memReq
	r.mem *= 1 + memMargin
	for p := range week {
		r.cpu[p] = r.curCPU
		if seen[p] {
			r.cpu[p] = min(r.curCPU, max(cpuFloor*r.curCPU, peak[p]*(1+cpuMargin)))
		}
	}
	return r, ""
}

// lastHour returns the workload's last sample in the trailing 8 days.
func lastHour(w *workload, hours int) (sample, bool) {
	last := -1
	for h := range w.rows {
		if h >= hours-history {
			last = max(last, h)
		}
	}
	return w.rows[last], last >= 0
}

func loadCurves(ns []node, dir string) (map[string]power.Curve, error) {
	out := map[string]power.Curve{}
	for _, n := range ns {
		if _, ok := out[n.curve]; ok {
			continue
		}
		f, err := os.Open(filepath.Join(dir, n.curve))
		if err != nil {
			return nil, err
		}
		c, _, err := power.ReadCurve(f)
		f.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n.curve, err)
		}
		out[n.curve] = c
	}
	return out, nil
}

// nodesNeeded counts the workers needed to hold the requests without
// headroom, most efficient first (power per CPU at half load, as the planner
// ranks them), or -1 if all of them cannot.
func nodesNeeded(ns []node, curves map[string]power.Curve, cpu, mem float64) int {
	order := slices.Clone(ns)
	eff := func(n node) float64 { return curves[n.curve].Watts(0.5) / n.cpus }
	slices.SortStableFunc(order, func(a, b node) int { return cmp.Compare(eff(a), eff(b)) })
	var c, m float64
	for i, n := range order {
		if c >= cpu && m >= mem {
			return i
		}
		c += n.cpus
		m += n.mem
	}
	if c >= cpu && m >= mem {
		return len(order)
	}
	return -1
}

func build(ws []*workload, hours int, ns []node, curveDir string, boot time.Duration) (*report, error) {
	curves, err := loadCurves(ns, curveDir)
	if err != nil {
		return nil, err
	}
	rep := &report{hours: hours, workloads: ws, nodes: ns, recs: map[string]*recommendation{},
		excluded: map[string]string{}, current: map[string]sample{}}
	for _, w := range ws {
		s, running := lastHour(w, hours)
		if !running {
			rep.excluded[w.name] = "not running in the last 8 days"
			continue
		}
		rep.current[w.name] = s
		if r, why := recommend(w, hours); r != nil {
			rep.recs[w.name] = r
		} else {
			rep.excluded[w.name] = why
		}
	}

	// Totals per hour of the week: workloads without a recommendation keep
	// their current requests.
	var cur, rec [week]float64
	for name, s := range rep.current {
		rep.curMem += s.memReq
		if r := rep.recs[name]; r != nil {
			rep.recMem += r.mem
		} else {
			rep.recMem += s.memReq
		}
		for p := range week {
			cur[p] += s.cpuReq
			if r := rep.recs[name]; r != nil {
				rec[p] += r.cpu[p]
			} else {
				rec[p] += s.cpuReq
			}
		}
	}
	for p := range week {
		if rec[p] > rec[rep.peak] {
			rep.peak = p
		}
	}
	rep.curCPU, rep.recCPU = cur[rep.peak], rec[rep.peak]
	rep.nodesCur = nodesNeeded(ns, curves, rep.curCPU, rep.curMem)
	rep.nodesRec = nodesNeeded(ns, curves, rep.recCPU, rep.recMem)

	var kept []string
	for _, n := range ns {
		if n.localPod != "" {
			kept = append(kept, n.name)
		}
	}
	switch {
	case len(kept) > 0:
		rep.energyNote = fmt.Sprintf("not estimated: %d node(s) stay on for node-local storage, and the simulator cannot yet hold a node on", len(kept))
	case hours <= week:
		rep.energyNote = fmt.Sprintf("not estimated: %d hours of history; the first week trains, so more than %d are needed", hours, week)
	default:
		e, err := estimate(ws, rep.recs, hours, ns, curves, boot)
		if err != nil {
			return nil, err
		}
		rep.energy = e
	}
	return rep, nil
}

// series returns the cluster's hourly totals for the three arms: current
// requests (B0 and T), requests rightsized out of sample (T+RW), and usage.
// Rightsized requests learn from earlier hours only, as wattproof-traceprep
// does, so the first week holds the current requests.
type series struct{ cpuReq, memReq, cpuShaped, memTrail, cpuUse, memUse []float64 }

func clusterSeries(ws []*workload, recs map[string]*recommendation, hours int) series {
	s := series{make([]float64, hours), make([]float64, hours), make([]float64, hours),
		make([]float64, hours), make([]float64, hours), make([]float64, hours)}
	for _, w := range ws {
		peak, req, floors := make([]float64, hours), make([]float64, hours), make([]float64, hours)
		use, mreq := make([]float64, hours), make([]float64, hours)
		for h, r := range w.rows {
			peak[h], req[h], floors[h] = r.cpuPeak, r.cpuReq, cpuFloor*r.cpuReq
			use[h], mreq[h] = r.memUse, r.memReq
		}
		shaped, trail := req, mreq
		if recs[w.name] != nil {
			shaped = sim.ShapeLearned(peak, req, floors, 1, week, cpuMargin)
			trail = sim.VPALearned(use, mreq, day, history, 1, memMargin)
		}
		for h, r := range w.rows {
			s.cpuReq[h] += r.cpuReq
			s.memReq[h] += r.memReq
			s.cpuUse[h] += r.cpuUse
			s.memUse[h] += r.memUse
			s.cpuShaped[h] += shaped[h]
			s.memTrail[h] += trail[h]
		}
	}
	return s
}

func specs(ns []node, curves map[string]power.Curve, boot time.Duration) []sim.NodeSpec {
	var out []sim.NodeSpec
	for _, n := range ns {
		c := curves[n.curve]
		out = append(out, sim.NodeSpec{Name: n.name, CPUs: n.cpus, Mem: n.mem, Power: power.Node{
			Curve: c, OffWatts: offWatts,
			Transition: power.Transition{
				Shutdown: time.Minute, ShutdownJoules: 60 * c.Idle(),
				Boot: boot, BootJoules: boot.Seconds() * c.Watts(bootLoad),
			},
		}})
	}
	return out
}

// estimate runs B0 (always on), T (power state alone) and T+RW (power state
// with rightsizing) on the cluster's nodes, the first week training.
func estimate(ws []*workload, recs map[string]*recommendation, hours int, ns []node, curves map[string]power.Curve, boot time.Duration) (*energyResult, error) {
	s := clusterSeries(ws, recs, hours)
	m := func(v []float64) []float64 { return sim.Interpolate(v, hourSteps) }
	usage, memUse := m(s.cpuUse), m(s.memUse)
	planner := func(label string) *sim.Planner {
		p := sim.NewPlanner(sim.SeasonalNaive{Period: day * hourSteps})
		p.MemForecast = sim.SeasonalNaive{Period: day * hourSteps}
		p.Label = label
		return p
	}
	arms := []struct {
		p        sim.Policy
		cpu, mem []float64
	}{
		{sim.AlwaysOn{}, m(s.cpuReq), m(s.memReq)},
		{planner("T"), m(s.cpuReq), m(s.memReq)},
		{planner("T+RW"), m(s.cpuShaped), m(s.memTrail)},
	}
	var res []sim.Result
	for _, a := range arms {
		r, err := sim.Run(sim.Config{Step: time.Minute, Nodes: specs(ns, curves, boot), Demand: a.cpu, Usage: usage,
			MemDemand: a.mem, MemUsage: memUse, MinNodes: minNodes, Score: week * hourSteps}, a.p)
		if err != nil {
			return nil, err
		}
		if r.Rejected != 0 {
			return nil, fmt.Errorf("%s: %d power-offs rejected by the invariants", r.Policy, r.Rejected)
		}
		res = append(res, r)
	}
	return &energyResult{days: float64(hours-week) / day, b0: res[0], t: res[1], trw: res[2]}, nil
}

func (r *report) write(w io.Writer) {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	fmt.Fprintln(bw, "wattproof-report: model estimates from requests and usage, not measurements. It changes nothing.")
	fmt.Fprintf(bw, "Input: %d workloads over %d hours (%.1f days); %d nodes.\n\n", len(r.workloads), r.hours, float64(r.hours)/day, len(r.nodes))

	fmt.Fprintln(bw, "Recommendations. CPU per hour of the week: the highest 5 minutes seen in that hour, +20%,")
	fmt.Fprintln(bw, "between 10% and 100% of the current request. Memory: the last 8 days' maximum, +15% (ADR-0013).")
	tw := tabwriter.NewWriter(bw, 0, 0, 2, ' ', tabwriter.AlignRight)
	fmt.Fprintln(tw, "workload\tCPU now\tCPU recommended, min\tmax\tmemory now\tmemory recommended\t")
	for _, wl := range r.workloads {
		rec := r.recs[wl.name]
		if rec == nil {
			continue
		}
		fmt.Fprintf(tw, "%s\t%.3g\t%.3g\t%.3g\t%.3g\t%.3g\t\n", wl.name, rec.curCPU, slices.Min(rec.cpu[:]), slices.Max(rec.cpu[:]), rec.curMem, rec.mem)
	}
	tw.Flush()
	var ratios []float64
	for _, rec := range r.recs {
		if rec.curCPU > 0 {
			ratios = append(ratios, slices.Max(rec.cpu[:])/rec.curCPU)
		}
	}
	if len(ratios) > 0 {
		slices.Sort(ratios)
		q := func(f float64) float64 { return ratios[min(len(ratios)-1, int(f*float64(len(ratios))))] }
		fmt.Fprintf(bw, "Highest recommended CPU over the current request, per workload: 10th percentile %.2f, median %.2f, 90th %.2f (%d workloads).\n",
			q(0.1), q(0.5), q(0.9), len(ratios))
	}
	if len(r.excluded) > 0 {
		// By reason; a short history's hours differ per workload, so they
		// group under their common prefix.
		const shown = 10
		byReason := map[string][]string{}
		var reasons []string
		for _, wl := range r.workloads {
			why, ok := r.excluded[wl.name]
			if !ok {
				continue
			}
			key, _, _ := strings.Cut(why, ":")
			if byReason[key] == nil {
				reasons = append(reasons, key)
			}
			if why != key {
				byReason[key] = append(byReason[key], fmt.Sprintf("%s (%s)", wl.name, why))
			} else {
				byReason[key] = append(byReason[key], wl.name)
			}
		}
		fmt.Fprintln(bw, "\nNo recommendation:")
		for _, key := range reasons {
			names := byReason[key]
			fmt.Fprintf(bw, "  %s: %d workloads\n", key, len(names))
			for _, n := range names[:min(shown, len(names))] {
				fmt.Fprintf(bw, "    %s\n", n)
			}
			if len(names) > shown {
				fmt.Fprintf(bw, "    and %d more\n", len(names)-shown)
			}
		}
	}

	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	fmt.Fprintf(bw, "\nAt the peak hour of the week (hour %d, %s %02d:00 if hour 0 is Monday 00:00):\n", r.peak, days[r.peak/day], r.peak%day)
	freed := func(a, b float64) string {
		if a <= 0 {
			return "-"
		}
		if b > a {
			return fmt.Sprintf("raised by %.3g (%.1f%%)", b-a, 100*(b-a)/a)
		}
		return fmt.Sprintf("freed %.3g (%.1f%%)", a-b, 100*(a-b)/a)
	}
	fmt.Fprintf(bw, "  requested CPU     %.4g -> %.4g, %s\n", r.curCPU, r.recCPU, freed(r.curCPU, r.recCPU))
	fmt.Fprintf(bw, "  requested memory  %.4g -> %.4g, %s\n", r.curMem, r.recMem, freed(r.curMem, r.recMem))
	count := func(n int) string {
		if n < 0 {
			return "do not fit"
		}
		return strconv.Itoa(n)
	}
	fmt.Fprintf(bw, "  nodes needed, most efficient first, no headroom: %s -> %s of %d\n", count(r.nodesCur), count(r.nodesRec), len(r.nodes))

	fmt.Fprintln(bw, "\nEnergy, simulated on the nodes' power curves (published curves until measured ones exist):")
	if r.energy == nil {
		fmt.Fprintf(bw, "  %s.\n", r.energyNote)
	} else {
		e := r.energy
		fmt.Fprintf(bw, "  over the last %.1f days (the first week trains; boot as given, off %g W):\n", e.days, offWatts)
		tw = tabwriter.NewWriter(bw, 0, 0, 2, ' ', tabwriter.AlignRight)
		fmt.Fprintln(tw, "arm\tkWh\tagainst B0\tpending min\tmemory over min\tpower-offs\t")
		for _, a := range []struct {
			label string
			r     sim.Result
		}{{"B0, always on", e.b0}, {"T, power state alone", e.t}, {"T+RW, with rightsizing", e.trw}} {
			fmt.Fprintf(tw, "%s\t%.1f\t%+.1f%%\t%.0f\t%.0f\t%d\t\n", a.label, a.r.KWh(), 100*(a.r.Joules/e.b0.Joules-1),
				a.r.ShortfallMinutes, a.r.MemOverloadMinutes, a.r.PowerOffs)
		}
		tw.Flush()
	}
	var kept []string
	for _, n := range r.nodes {
		if n.localPod != "" {
			kept = append(kept, fmt.Sprintf("  %s: %s", n.name, n.localPod))
		}
	}
	if len(kept) > 0 {
		fmt.Fprintln(bw, "\nNodes kept on for node-local storage (invariant 1), with the pod:")
		fmt.Fprintln(bw, strings.Join(kept, "\n"))
	}
}

func (r *report) writeRecommendations(w io.Writer) error {
	cw := csv.NewWriter(w)
	cw.Write([]string{"workload", "hour_of_week", "cpu_request", "mem_request"})
	for _, wl := range r.workloads {
		rec := r.recs[wl.name]
		if rec == nil {
			continue
		}
		for p := range week {
			cw.Write([]string{wl.name, strconv.Itoa(p), strconv.FormatFloat(rec.cpu[p], 'g', 6, 64), strconv.FormatFloat(rec.mem, 'g', 6, 64)})
		}
	}
	cw.Flush()
	return cw.Error()
}
