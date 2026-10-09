// Package sim is a discrete-time simulation of a cluster's workers powering
// on and off under a policy. It exists for experiment design and testing
// (ADR-0011): its numbers are labelled "simulation" and are never a measured
// saving.
//
// The model is fluid. Requested CPU is one quantity spread over powered nodes,
// so fragmentation and pod sizes are ignored. Requested memory, when given,
// rides along with it: every pod asks for the same memory per requested CPU,
// so a node holds at most min(CPUs, Mem ÷ that ratio) of requested CPU, and
// whichever runs out first limits packing. Usage is another quantity: each
// node runs its share of it in proportion to the requests placed on it, and
// its power comes from its measured curve at that load.
package sim

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/wattproof/wattproof/internal/power"
)

// Phase is a node's power state.
type Phase int

const (
	// On means powered and Ready.
	On Phase = iota
	// ShuttingDown means drained and switching off.
	ShuttingDown
	// Off means drawing only standby power.
	Off
	// Booting means powered on but not yet Ready.
	Booting
)

func (p Phase) String() string {
	return [...]string{"on", "shutting down", "off", "booting"}[p]
}

// NodeSpec describes one worker.
type NodeSpec struct {
	Name string
	CPUs float64
	// Mem is the memory capacity, in the unit of Config.MemDemand. It is
	// ignored when the run has no memory demand.
	Mem   float64
	Power power.Node
}

// Eff is the requested CPU the node can hold when pods request k units of
// memory per requested CPU: its CPUs, or less if its memory runs out first.
// k = 0 means memory is not modelled.
func (n NodeSpec) Eff(k float64) float64 {
	if k <= 0 {
		return n.CPUs
	}
	return min(n.CPUs, n.Mem/k)
}

// Node is a worker's state during a run.
type Node struct {
	Spec  NodeSpec
	Phase Phase
	// Left is the time remaining in ShuttingDown or Booting.
	Left time.Duration
	// WantOn means boot as soon as the shutdown completes.
	WantOn bool
}

// Powered reports whether the node draws more than standby power.
func (n Node) Powered() bool { return n.Phase != Off }

// Staying reports whether the node is powered and will stay so: a node that
// is shutting down still draws power but no longer counts towards the
// minimum.
func (n Node) Staying() bool {
	return n.Phase == On || n.Phase == Booting || (n.Phase == ShuttingDown && n.WantOn)
}

// Packing is how the scheduler spreads requests over Ready nodes.
type Packing int

const (
	// Spread fills every Ready node to the same fraction, as kube-scheduler's
	// default LeastAllocated scoring tends to.
	Spread Packing = iota
	// Pack fills Ready nodes one after another, in node order, as
	// MostAllocated scoring tends to.
	Pack
)

// State is what a policy sees at each step.
type State struct {
	Step    int
	StepLen time.Duration
	Nodes   []Node
	// History is the requested CPU from step 0 up to and including Step.
	History []float64
	// MemHistory is the requested memory over the same steps, or nil when
	// memory is not modelled.
	MemHistory []float64
	MinNodes   int
}

// Demand is the requested CPU now.
func (s *State) Demand() float64 { return s.History[len(s.History)-1] }

// MemDemand is the requested memory now, or 0 when memory is not modelled.
func (s *State) MemDemand() float64 {
	if s.MemHistory == nil {
		return 0
	}
	return s.MemHistory[len(s.MemHistory)-1]
}

// Ratio is the memory requested per requested CPU now (see NodeSpec.Eff).
func (s *State) Ratio() float64 { return ratio(s.Demand(), s.MemDemand()) }

func ratio(cpu, mem float64) float64 {
	switch {
	case mem <= 0:
		return 0
	case cpu <= 0:
		return math.Inf(1)
	}
	return mem / cpu
}

// Policy decides which nodes to power on and off. Decide must not modify
// the state.
type Policy interface {
	Name() string
	Packing() Packing
	Decide(s *State) (on, off []int)
}

// Config is one simulated run.
type Config struct {
	Step  time.Duration
	Nodes []NodeSpec
	// Demand is the requested CPU at each step.
	Demand []float64
	// Usage is the CPU in use at each step. Requests and usage differ:
	// a service with fixed replicas keeps its requests all night while its
	// usage follows traffic. Nil means Demand × UseRatio.
	Usage []float64
	// UseRatio is CPU usage divided by CPU requests, used when Usage is nil.
	UseRatio float64
	// MemDemand is the requested memory at each step. Nil means memory is
	// not modelled and never limits packing; otherwise every node needs Mem.
	MemDemand []float64
	// MemUsage is the memory in use at each step, or nil. Like CPU usage, each
	// node holds its share in proportion to the requests placed on it.
	MemUsage []float64
	// MinNodes is the fewest powered workers allowed (invariant 3).
	MinNodes int
	// BaseWatts is drawn throughout and is the same in every arm: the
	// control plane, for example.
	BaseWatts float64
	// Score is the first step that counts. Earlier steps are warm-up.
	Score int
}

// Result is the outcome of a run, over the scored steps.
type Result struct {
	Policy string
	Joules float64
	// NodeHours counts hours workers spent powered (not Off).
	NodeHours float64
	// ShortfallMinutes counts minutes in which requested CPU or memory
	// exceeded the capacity of Ready nodes, so pods were pending.
	ShortfallMinutes float64
	// OverloadMinutes counts minutes in which some node's CPU usage exceeded
	// its capacity, so pods were throttled. Requests below usage allow this.
	OverloadMinutes float64
	// MemOverloadMinutes counts minutes in which some node's memory usage
	// exceeded its memory, which in a real cluster means evictions or pods
	// killed for lack of memory. Only counted when MemUsage is given.
	MemOverloadMinutes float64
	// PowerOffs counts shutdowns started.
	PowerOffs int
	// Rejected counts power-offs the simulator refused: they would have left
	// fewer than MinNodes powered or pods with nowhere to go (invariants 1
	// and 3). A correct policy has none.
	Rejected int
	// MinPowered is the fewest workers powered at any scored step.
	MinPowered int
	// PoweredHours is NodeHours per node, in node order.
	PoweredHours []float64
}

// KWh returns the energy in kilowatt-hours.
func (r Result) KWh() float64 { return r.Joules / 3.6e6 }

// ErrConfig means the configuration cannot be run.
var ErrConfig = errors.New("sim: bad config")

// Run simulates the policy over the demand. Every node starts On.
func Run(cfg Config, p Policy) (Result, error) {
	if cfg.Step <= 0 || len(cfg.Nodes) == 0 || len(cfg.Demand) == 0 ||
		cfg.Score < 0 || cfg.Score >= len(cfg.Demand) || cfg.MinNodes > len(cfg.Nodes) ||
		(cfg.Usage != nil && len(cfg.Usage) != len(cfg.Demand)) ||
		(cfg.MemDemand != nil && len(cfg.MemDemand) != len(cfg.Demand)) ||
		(cfg.MemUsage != nil && (cfg.MemDemand == nil || len(cfg.MemUsage) != len(cfg.Demand))) {
		return Result{}, fmt.Errorf("%w: %+v", ErrConfig, cfg)
	}
	nodes := make([]Node, len(cfg.Nodes))
	for i, spec := range cfg.Nodes {
		if spec.CPUs <= 0 || (cfg.MemDemand != nil && spec.Mem <= 0) {
			return Result{}, fmt.Errorf("%w: node %d has %v CPUs and %v memory", ErrConfig, i, spec.CPUs, spec.Mem)
		}
		nodes[i] = Node{Spec: spec, Phase: On}
	}
	res := Result{Policy: p.Name(), MinPowered: len(nodes), PoweredHours: make([]float64, len(nodes))}
	dt := cfg.Step.Seconds()

	for step, demand := range cfg.Demand {
		scored := step >= cfg.Score
		advance(nodes, cfg.Step)
		var mem float64
		var memHist []float64
		if cfg.MemDemand != nil {
			mem, memHist = cfg.MemDemand[step], cfg.MemDemand[:step+1]
		}
		k := ratio(demand, mem)

		on, off := p.Decide(&State{
			Step: step, StepLen: cfg.Step, Nodes: append([]Node(nil), nodes...),
			History: cfg.Demand[:step+1], MemHistory: memHist, MinNodes: cfg.MinNodes,
		})
		for _, i := range on {
			powerOn(&nodes[i])
		}
		for _, i := range off {
			if !canPowerOff(nodes, i, demand, mem, cfg.MinNodes) {
				if scored {
					res.Rejected++
				}
				continue
			}
			powerOff(&nodes[i])
			if scored {
				res.PowerOffs++
			}
		}

		requests := Place(nodes, demand, k, p.Packing())
		if !fits(nodes, -1, demand, mem) && scored {
			res.ShortfallMinutes += cfg.Step.Minutes()
		}
		if !scored {
			continue
		}
		// Each running pod uses its share of the usage, in proportion to its
		// requests. Pending pods use nothing.
		usage := demand * cfg.UseRatio
		if cfg.Usage != nil {
			usage = cfg.Usage[step]
		}
		var memUsage float64
		if cfg.MemUsage != nil {
			memUsage = cfg.MemUsage[step]
		}
		powered := 0
		overloaded, memOverloaded := false, false
		res.Joules += cfg.BaseWatts * dt
		for i, n := range nodes {
			var used float64
			if demand > 0 {
				used = usage * requests[i] / demand
			}
			overloaded = overloaded || used > n.Spec.CPUs+1e-9
			if demand > 0 && memUsage > 0 {
				memOverloaded = memOverloaded || memUsage*requests[i]/demand > n.Spec.Mem+1e-9
			}
			res.Joules += watts(n, used/n.Spec.CPUs) * dt
			if n.Powered() {
				powered++
				res.NodeHours += cfg.Step.Hours()
				res.PoweredHours[i] += cfg.Step.Hours()
			}
		}
		res.MinPowered = min(res.MinPowered, powered)
		if overloaded {
			res.OverloadMinutes += cfg.Step.Minutes()
		}
		if memOverloaded {
			res.MemOverloadMinutes += cfg.Step.Minutes()
		}
	}
	return res, nil
}

// advance moves transitions forward by one step.
func advance(nodes []Node, step time.Duration) {
	for i := range nodes {
		n := &nodes[i]
		if n.Phase != ShuttingDown && n.Phase != Booting {
			continue
		}
		n.Left -= step
		if n.Left > 0 {
			continue
		}
		if n.Phase == Booting {
			n.Phase = On
			continue
		}
		n.Phase = Off
		if n.WantOn {
			powerOn(n)
		}
	}
}

func powerOn(n *Node) {
	switch n.Phase {
	case Off:
		n.Phase, n.Left, n.WantOn = Booting, n.Spec.Power.Boot, false
		if n.Left <= 0 {
			n.Phase = On
		}
	case ShuttingDown:
		n.WantOn = true
	}
}

func powerOff(n *Node) {
	n.Phase, n.Left, n.WantOn = ShuttingDown, n.Spec.Power.Shutdown, false
	if n.Left <= 0 {
		n.Phase = Off
	}
}

// canPowerOff enforces invariants 1 and 3: the node is On, enough workers stay
// powered, and its pods fit on the remaining Ready nodes.
func canPowerOff(nodes []Node, i int, demand, mem float64, minNodes int) bool {
	if nodes[i].Phase != On {
		return false
	}
	staying := 0
	for _, n := range nodes {
		if n.Staying() {
			staying++
		}
	}
	return staying-1 >= minNodes && fits(nodes, i, demand, mem)
}

// fits reports whether the requests fit on the Ready nodes other than skip
// (-1 skips none).
func fits(nodes []Node, skip int, demand, mem float64) bool {
	k := ratio(demand, mem)
	var eff, m float64
	for j, n := range nodes {
		if n.Phase == On && j != skip {
			eff += n.Spec.Eff(k)
			m += n.Spec.Mem
		}
	}
	return demand <= eff+1e-9 && (mem <= 0 || mem <= m+1e-9)
}

// readyEff is the requested CPU the Ready nodes can hold at memory ratio k.
func readyEff(nodes []Node, k float64) float64 {
	var c float64
	for _, n := range nodes {
		if n.Phase == On {
			c += n.Spec.Eff(k)
		}
	}
	return c
}

// Place returns the requested CPU on each node, for pods that request k units
// of memory per requested CPU (0: memory not modelled). Only On nodes receive
// any. Demand beyond their capacity stays pending.
func Place(nodes []Node, demand, k float64, p Packing) []float64 {
	req := make([]float64, len(nodes))
	ready := readyEff(nodes, k)
	if ready == 0 || demand <= 0 {
		return req
	}
	switch p {
	case Spread:
		frac := min(demand/ready, 1)
		for i, n := range nodes {
			if n.Phase == On {
				req[i] = n.Spec.Eff(k) * frac
			}
		}
	case Pack:
		left := demand
		for i, n := range nodes {
			if n.Phase == On && left > 0 {
				req[i] = min(n.Spec.Eff(k), left)
				left -= req[i]
			}
		}
	}
	return req
}

// watts is a node's power draw at load in its current phase. Transitions draw
// their energy evenly over their duration.
func watts(n Node, load float64) float64 {
	p := n.Spec.Power
	switch n.Phase {
	case On:
		return p.Curve.Watts(load)
	case Booting:
		return p.BootJoules / p.Boot.Seconds()
	case ShuttingDown:
		return p.ShutdownJoules / p.Shutdown.Seconds()
	default:
		return p.OffWatts
	}
}
