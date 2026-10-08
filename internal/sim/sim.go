// Package sim is a discrete-time simulation of a cluster's workers powering
// on and off under a policy. It exists for experiment design and testing
// (ADR-0011): its numbers are labelled "simulation" and are never a measured
// saving.
//
// The model is fluid. Requested CPU is one quantity spread over powered nodes,
// so fragmentation, pod sizes and memory are ignored. A node's load is its
// share of requested CPU times the usage-to-request ratio, and its power comes
// from its measured curve.
package sim

import (
	"errors"
	"fmt"
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
	Name  string
	CPUs  float64
	Power power.Node
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
	History  []float64
	MinNodes int
}

// Demand is the requested CPU now.
func (s *State) Demand() float64 { return s.History[len(s.History)-1] }

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
	// UseRatio is CPU usage divided by CPU requests.
	UseRatio float64
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
	// ShortfallMinutes counts minutes in which requested CPU exceeded the
	// capacity of Ready nodes, so pods were pending.
	ShortfallMinutes float64
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
		cfg.Score < 0 || cfg.Score >= len(cfg.Demand) || cfg.MinNodes > len(cfg.Nodes) {
		return Result{}, fmt.Errorf("%w: %+v", ErrConfig, cfg)
	}
	nodes := make([]Node, len(cfg.Nodes))
	for i, spec := range cfg.Nodes {
		if spec.CPUs <= 0 {
			return Result{}, fmt.Errorf("%w: node %d has %v CPUs", ErrConfig, i, spec.CPUs)
		}
		nodes[i] = Node{Spec: spec, Phase: On}
	}
	res := Result{Policy: p.Name(), MinPowered: len(nodes), PoweredHours: make([]float64, len(nodes))}
	dt := cfg.Step.Seconds()

	for step, demand := range cfg.Demand {
		scored := step >= cfg.Score
		advance(nodes, cfg.Step)

		on, off := p.Decide(&State{
			Step: step, StepLen: cfg.Step, Nodes: append([]Node(nil), nodes...),
			History: cfg.Demand[:step+1], MinNodes: cfg.MinNodes,
		})
		for _, i := range on {
			powerOn(&nodes[i])
		}
		for _, i := range off {
			if !canPowerOff(nodes, i, demand, cfg.MinNodes) {
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

		requests := Place(nodes, demand, p.Packing())
		if demand > readyCPUs(nodes)+1e-9 && scored {
			res.ShortfallMinutes += cfg.Step.Minutes()
		}
		if !scored {
			continue
		}
		powered := 0
		res.Joules += cfg.BaseWatts * dt
		for i, n := range nodes {
			res.Joules += watts(n, requests[i]*cfg.UseRatio/n.Spec.CPUs) * dt
			if n.Powered() {
				powered++
				res.NodeHours += cfg.Step.Hours()
				res.PoweredHours[i] += cfg.Step.Hours()
			}
		}
		res.MinPowered = min(res.MinPowered, powered)
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
func canPowerOff(nodes []Node, i int, demand float64, minNodes int) bool {
	if nodes[i].Phase != On {
		return false
	}
	staying := 0
	for _, n := range nodes {
		if n.Staying() {
			staying++
		}
	}
	return staying-1 >= minNodes && readyCPUs(nodes)-nodes[i].Spec.CPUs >= demand-1e-9
}

func readyCPUs(nodes []Node) float64 {
	var c float64
	for _, n := range nodes {
		if n.Phase == On {
			c += n.Spec.CPUs
		}
	}
	return c
}

// Place returns the requested CPU on each node. Only On nodes receive any.
// Demand beyond their capacity stays pending.
func Place(nodes []Node, demand float64, p Packing) []float64 {
	req := make([]float64, len(nodes))
	ready := readyCPUs(nodes)
	if ready == 0 || demand <= 0 {
		return req
	}
	switch p {
	case Spread:
		frac := min(demand/ready, 1)
		for i, n := range nodes {
			if n.Phase == On {
				req[i] = n.Spec.CPUs * frac
			}
		}
	case Pack:
		left := demand
		for i, n := range nodes {
			if n.Phase == On && left > 0 {
				req[i] = min(n.Spec.CPUs, left)
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
