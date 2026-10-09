// Package power models one server's wall power: what it draws at a given
// load, what it draws when off, and what switching it off and on costs. The
// quantities and the break-even rule are defined in docs/architecture.md,
// "Where the energy goes".
package power

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// Point is the measured wall power at one load. Load is the fraction of the
// server's capacity in use, from 0 (active idle) to 1 (full load).
type Point struct {
	Load  float64
	Watts float64
}

// Curve is a server's wall power over load, from measured points. Between
// points, power is interpolated linearly. It is never extrapolated: a curve
// starts at load 0 and ends at load 1.
type Curve struct {
	points []Point
}

// ErrBadCurve means the points cannot form a curve.
var ErrBadCurve = errors.New("power: bad curve")

// NewCurve checks the points and returns the curve. Loads must rise strictly
// from 0 to 1, and no power may be negative.
func NewCurve(points []Point) (Curve, error) {
	if len(points) < 2 {
		return Curve{}, fmt.Errorf("%w: need at least 2 points, got %d", ErrBadCurve, len(points))
	}
	if points[0].Load != 0 || points[len(points)-1].Load != 1 {
		return Curve{}, fmt.Errorf("%w: loads must run from 0 to 1", ErrBadCurve)
	}
	for i, p := range points {
		if p.Watts < 0 || math.IsNaN(p.Watts) {
			return Curve{}, fmt.Errorf("%w: point %d has power %v", ErrBadCurve, i, p.Watts)
		}
		if i > 0 && !(p.Load > points[i-1].Load) {
			return Curve{}, fmt.Errorf("%w: loads not strictly increasing at point %d", ErrBadCurve, i)
		}
	}
	return Curve{points: append([]Point(nil), points...)}, nil
}

// Watts returns the power at load. Loads outside [0, 1] are clamped.
func (c Curve) Watts(load float64) float64 {
	pts := c.points
	if load <= 0 {
		return pts[0].Watts
	}
	if load >= 1 {
		return pts[len(pts)-1].Watts
	}
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		if load <= b.Load {
			return a.Watts + (b.Watts-a.Watts)*(load-a.Load)/(b.Load-a.Load)
		}
	}
	return pts[len(pts)-1].Watts
}

// Idle is the power at load 0.
func (c Curve) Idle() float64 { return c.points[0].Watts }

// Peak is the power at load 1.
func (c Curve) Peak() float64 { return c.points[len(c.points)-1].Watts }

// WithIdleShare returns the curve with its idle power moved to share × peak,
// keeping the peak and the curve's shape in between: every point keeps its
// position between idle and peak. It is for sensitivity runs, because the idle
// share is the number a published curve transfers worst to another server and
// workload (docs/research/trace-analysis-plan.md).
func (c Curve) WithIdleShare(share float64) (Curve, error) {
	idle, peak := c.Idle(), c.Peak()
	if share < 0 || share > 1 || peak <= idle {
		return Curve{}, fmt.Errorf("%w: idle share %v of a curve from %v W to %v W", ErrBadCurve, share, idle, peak)
	}
	newIdle := share * peak
	pts := make([]Point, len(c.points))
	for i, p := range c.points {
		pts[i] = Point{Load: p.Load, Watts: newIdle + (p.Watts-idle)*(peak-newIdle)/(peak-idle)}
	}
	return NewCurve(pts)
}

// Transition is the cost of switching a node off and on again.
type Transition struct {
	// Shutdown is the time from the end of the drain until the node is off.
	Shutdown time.Duration
	// ShutdownJoules is the energy drawn during Shutdown.
	ShutdownJoules float64
	// Boot is the time from power-on until the node is Ready.
	Boot time.Duration
	// BootJoules is the energy drawn during Boot.
	BootJoules float64
}

// Node is the power model of one server.
type Node struct {
	Curve Curve
	// OffWatts is the standby power when the node is off, mostly its BMC.
	OffWatts float64
	Transition
}

// BreakEven returns the shortest off-time, from the end of shutdown to the
// start of boot, for which switching the node off uses less energy than
// leaving it idle. During shutdown and boot the node would have drawn idle
// power anyway, so only the energy above idle counts as a cost:
//
//	t = (E_shutdown + E_boot - P_idle × (t_shutdown + t_boot)) / (P_idle - P_off)
//
// ok is false when switching off never pays, because the node draws no more
// when idle than when off.
func (n Node) BreakEven() (t time.Duration, ok bool) {
	saving := n.Curve.Idle() - n.OffWatts // watts saved while off
	if saving <= 0 {
		return 0, false
	}
	transitions := n.Shutdown + n.Boot
	extra := n.ShutdownJoules + n.BootJoules - n.Curve.Idle()*transitions.Seconds()
	if extra <= 0 {
		return 0, true
	}
	return time.Duration(extra / saving * float64(time.Second)), true
}

// MinCycle returns the shortest time from the start of shutdown until the
// node is Ready again for which the switch saves energy: shutdown, the
// break-even off-time, and boot.
func (n Node) MinCycle() (time.Duration, bool) {
	t, ok := n.BreakEven()
	if !ok {
		return 0, false
	}
	return n.Shutdown + t + n.Boot, true
}

// ReadCurve reads a curve from CSV: a header line "load,watts", then one point
// per line. Lines starting with '#' are comments. A comment "# cpus: N" gives
// the server's logical CPU count, returned as cpus (0 if absent).
func ReadCurve(r io.Reader) (c Curve, cpus int, err error) {
	var pts []Point
	header := false
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		switch {
		case text == "":
			continue
		case strings.HasPrefix(text, "#"):
			if v, found := strings.CutPrefix(strings.TrimSpace(text[1:]), "cpus:"); found {
				if cpus, err = strconv.Atoi(strings.TrimSpace(v)); err != nil {
					return Curve{}, 0, fmt.Errorf("line %d: cpus: %w", line, err)
				}
			}
			continue
		case !header:
			if text != "load,watts" {
				return Curve{}, 0, fmt.Errorf("line %d: want header \"load,watts\", got %q", line, text)
			}
			header = true
			continue
		}
		f := strings.Split(text, ",")
		if len(f) != 2 {
			return Curve{}, 0, fmt.Errorf("line %d: want 2 fields, got %d", line, len(f))
		}
		load, err1 := strconv.ParseFloat(strings.TrimSpace(f[0]), 64)
		watts, err2 := strconv.ParseFloat(strings.TrimSpace(f[1]), 64)
		if err := errors.Join(err1, err2); err != nil {
			return Curve{}, 0, fmt.Errorf("line %d: %w", line, err)
		}
		pts = append(pts, Point{Load: load, Watts: watts})
	}
	if err := sc.Err(); err != nil {
		return Curve{}, 0, err
	}
	c, err = NewCurve(pts)
	return c, cpus, err
}
