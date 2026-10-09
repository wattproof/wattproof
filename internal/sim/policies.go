package sim

import (
	"cmp"
	"slices"
	"time"
)

// AlwaysOn is B0: stock Kubernetes, every node powered all the time.
type AlwaysOn struct{}

func (AlwaysOn) Name() string                  { return "B0" }
func (AlwaysOn) Packing() Packing              { return Spread }
func (AlwaysOn) Decide(*State) (on, off []int) { return nil, nil }

// Autoscaler is B1: tight packing plus Cluster Autoscaler's scale-down and
// scale-up rules, approximated. A node is unneeded when its requests are below
// Threshold of its capacity (the larger of its CPU and memory utilisation, as
// Cluster Autoscaler takes) and fit on the other Ready nodes; after Unneeded
// in that state it is powered off. Pending demand powers on nodes in node
// order, which is not efficiency-aware. No scale-down runs within
// DelayAfterAdd of a scale-up.
type Autoscaler struct {
	Threshold     float64
	Unneeded      time.Duration
	DelayAfterAdd time.Duration

	since   []int // step a node became unneeded, or -1
	lastAdd int
}

// NewAutoscaler returns B1 with Cluster Autoscaler's defaults: threshold 0.5,
// 10 minutes unneeded, 10 minutes delay after a scale-up.
func NewAutoscaler() *Autoscaler {
	return &Autoscaler{Threshold: 0.5, Unneeded: 10 * time.Minute, DelayAfterAdd: 10 * time.Minute}
}

func (*Autoscaler) Name() string     { return "B1" }
func (*Autoscaler) Packing() Packing { return Pack }

func (a *Autoscaler) Decide(s *State) (on, off []int) {
	if a.since == nil {
		a.since = make([]int, len(s.Nodes))
		for i := range a.since {
			a.since[i] = -1
		}
		a.lastAdd = -1 << 30
	}
	demand, k := s.Demand(), s.Ratio()

	// Scale up: pending demand that capacity on its way will not cover.
	var coming float64
	for _, n := range s.Nodes {
		if n.Phase == On || n.Phase == Booting || n.WantOn {
			coming += n.Spec.Eff(k)
		}
	}
	for i, n := range s.Nodes {
		if coming >= demand {
			break
		}
		if n.Phase == Off || (n.Phase == ShuttingDown && !n.WantOn) {
			on = append(on, i)
			coming += n.Spec.Eff(k)
		}
	}
	if len(on) > 0 {
		a.lastAdd = s.Step
	}

	// Scale down: least-utilised Ready nodes first, while the rest still
	// hold every request.
	req := Place(s.Nodes, demand, k, Pack)
	ready := readyEff(s.Nodes, k)
	// util is the larger of CPU and memory utilisation: requests over Eff.
	util := func(i int) float64 { return req[i] / s.Nodes[i].Spec.Eff(k) }
	powered := 0
	var cands []int
	for i, n := range s.Nodes {
		if n.Staying() {
			powered++
		}
		if n.Phase == On && util(i) < a.Threshold {
			cands = append(cands, i)
			continue
		}
		a.since[i] = -1
	}
	slices.SortStableFunc(cands, func(x, y int) int { return cmp.Compare(util(x), util(y)) })
	delayed := time.Duration(s.Step-a.lastAdd)*s.StepLen < a.DelayAfterAdd
	for _, i := range cands {
		if ready-s.Nodes[i].Spec.Eff(k) < demand {
			a.since[i] = -1
			continue
		}
		if a.since[i] < 0 {
			a.since[i] = s.Step
		}
		if delayed || powered-1 < s.MinNodes ||
			time.Duration(s.Step-a.since[i])*s.StepLen < a.Unneeded {
			continue
		}
		off = append(off, i)
		ready -= s.Nodes[i].Spec.Eff(k)
		powered--
		a.since[i] = -1
	}
	return on, off
}

// Planner is T: Wattproof's greedy planner (docs/architecture.md, Planner).
// It keeps the most efficient nodes powered until they cover the forecast
// demand plus headroom over the next boot time. It powers a node off only if
// the forecast says it will not be needed again within its break-even cycle
// plus a margin, and only once Ready nodes already cover that need. Within the
// powered set the stock scheduler spreads pods (ADR-0005).
type Planner struct {
	Forecast Forecaster
	// MemForecast predicts requested memory. Nil holds today's memory over
	// the lookahead; memory requests change slowly. Unused without memory.
	MemForecast Forecaster
	// Headroom is the fraction added to the forecast.
	Headroom float64
	// Lookahead is how far ahead the forecast is read for power-off decisions.
	Lookahead time.Duration
	// Margin is added to the break-even cycle (hysteresis).
	Margin time.Duration
	// MaxShutdowns limits nodes shutting down at once.
	MaxShutdowns int
	// Label names the arm; the default is "T".
	Label string
	// Scheduler is how pods spread within the powered set. The default,
	// Spread, is stock kube-scheduler; Pack models an operator who also
	// switches to MostAllocated scoring.
	Scheduler Packing
}

// NewPlanner returns T with the default parameters: 10% headroom, a 6-hour
// lookahead, a 5-minute margin and two shutdowns at a time.
func NewPlanner(f Forecaster) *Planner {
	return &Planner{Forecast: f, Headroom: 0.1, Lookahead: 6 * time.Hour,
		Margin: 5 * time.Minute, MaxShutdowns: 2}
}

func (p *Planner) Name() string {
	if p.Label != "" {
		return p.Label
	}
	return "T"
}

func (p *Planner) Packing() Packing { return p.Scheduler }

// Rank orders nodes by power per CPU at half load, most efficient first.
func Rank(nodes []Node) []int {
	r := make([]int, len(nodes))
	for i := range r {
		r[i] = i
	}
	eff := func(i int) float64 { return nodes[i].Spec.Power.Curve.Watts(0.5) / nodes[i].Spec.CPUs }
	slices.SortStableFunc(r, func(x, y int) int { return cmp.Compare(eff(x), eff(y)) })
	return r
}

func (p *Planner) Decide(s *State) (on, off []int) {
	var bootMax time.Duration
	for _, n := range s.Nodes {
		bootMax = max(bootMax, n.Spec.Power.Boot)
	}
	steps := func(d time.Duration) int { return int((d + s.StepLen - 1) / s.StepLen) }
	horizon := steps(bootMax) + 1
	look := max(steps(p.Lookahead), horizon)

	// need[h] and memNeed[h]: CPU and memory required h steps from now,
	// never below today's. Memory is compared with summed node memory, which
	// is exact when nodes share one memory-per-CPU ratio and approximate
	// otherwise; the simulator's own checks catch what slips through.
	need := make([]float64, look+1)
	memNeed := make([]float64, look+1)
	for h := range need {
		need[h] = max(p.Forecast.Forecast(s.History, h), s.Demand()) * (1 + p.Headroom)
		if s.MemHistory != nil {
			m := s.MemDemand()
			if p.MemForecast != nil {
				m = max(p.MemForecast.Forecast(s.MemHistory, h), m)
			}
			memNeed[h] = m * (1 + p.Headroom)
		}
	}
	// cover[h]: the most needed at any point in the next boot time from h.
	window := func(v []float64, h int) float64 { return slices.Max(v[h:min(h+horizon+1, len(v))]) }
	cover := func(h int) float64 { return window(need, h) }
	memCover := func(h int) float64 { return window(memNeed, h) }

	// The powered set: best-ranked nodes until they cover the need.
	rank := Rank(s.Nodes)
	inSet := make([]bool, len(s.Nodes))
	var cum, cumMem float64
	count := 0
	for _, i := range rank {
		if cum >= cover(0) && cumMem >= memCover(0) && count >= s.MinNodes {
			break
		}
		inSet[i] = true
		cum += s.Nodes[i].Spec.CPUs
		cumMem += s.Nodes[i].Spec.Mem
		count++
	}

	shutting := 0
	ready, readyMem := 0.0, 0.0
	for i, n := range s.Nodes {
		switch {
		case n.Phase == ShuttingDown && !n.WantOn:
			shutting++
		case n.Phase == On:
			ready += n.Spec.CPUs
			readyMem += n.Spec.Mem
		}
		if inSet[i] && (n.Phase == Off || (n.Phase == ShuttingDown && !n.WantOn)) {
			on = append(on, i)
		}
	}

	// Power off, worst-ranked first.
	var above, aboveMem float64 // capacity of nodes ranked better than the current one
	aboveOf := make([]float64, len(s.Nodes))
	aboveMemOf := make([]float64, len(s.Nodes))
	for _, i := range rank {
		aboveOf[i], aboveMemOf[i] = above, aboveMem
		above += s.Nodes[i].Spec.CPUs
		aboveMem += s.Nodes[i].Spec.Mem
	}
	for k := len(rank) - 1; k >= 0; k-- {
		i := rank[k]
		n := s.Nodes[i]
		if inSet[i] || n.Phase != On || shutting >= p.MaxShutdowns {
			continue
		}
		cycle, ok := n.Spec.Power.MinCycle()
		if !ok || ready-n.Spec.CPUs < cover(0) || readyMem-n.Spec.Mem < memCover(0) {
			continue
		}
		// First step at which better-ranked nodes alone no longer suffice.
		needed := len(need)
		for h := range need {
			if need[h] > aboveOf[i] || memNeed[h] > aboveMemOf[i] {
				needed = h
				break
			}
		}
		if time.Duration(needed)*s.StepLen < cycle+p.Margin {
			continue
		}
		off = append(off, i)
		ready -= n.Spec.CPUs
		readyMem -= n.Spec.Mem
		shutting++
	}
	return on, off
}
