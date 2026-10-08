// Package energy turns power and counter samples into energy, following the
// rules in docs/measurement.md: prefer the device's energy counter, integrate
// power only when there is none, and never interpolate across a gap.
package energy

import (
	"errors"
	"fmt"
	"time"
)

// MaxGap is the longest interval between two samples that may be integrated.
// Longer intervals are reported as gaps and contribute nothing.
const MaxGap = 5 * time.Second

// Point is one sample: power in watts for FromPower, or a cumulative counter
// in joules for FromCounter.
type Point struct {
	T time.Time
	V float64
}

// Gap is an interval between two consecutive samples that was too long to trust.
type Gap struct {
	From, To time.Time
}

// Duration returns the length of the gap.
func (g Gap) Duration() time.Duration { return g.To.Sub(g.From) }

// Result is the energy over a series of samples.
type Result struct {
	// Joules is the energy over the covered intervals.
	Joules float64
	// Span is the time from the first to the last sample.
	Span time.Duration
	// Gaps lists intervals that were not trusted.
	Gaps []Gap
}

// GapFraction is the share of Span that falls in gaps. A block whose gap
// fraction exceeds 1% is excluded from experiments.
func (r Result) GapFraction() float64 {
	if r.Span <= 0 {
		return 0
	}
	var total time.Duration
	for _, g := range r.Gaps {
		total += g.Duration()
	}
	return float64(total) / float64(r.Span)
}

var (
	// ErrNotIncreasing means sample times are not strictly increasing.
	ErrNotIncreasing = errors.New("energy: sample times not strictly increasing")
	// ErrCounterDecreased means an energy counter went backwards, e.g. after
	// a device reset. The series must be split at that point by the caller.
	ErrCounterDecreased = errors.New("energy: counter decreased")
)

// FromPower integrates power samples with the trapezoid rule. Intervals
// longer than maxGap are reported as gaps and not integrated.
func FromPower(points []Point, maxGap time.Duration) (Result, error) {
	var r Result
	if err := checkTimes(points); err != nil {
		return r, err
	}
	if len(points) < 2 {
		return r, nil
	}
	r.Span = points[len(points)-1].T.Sub(points[0].T)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		dt := b.T.Sub(a.T)
		if dt > maxGap {
			r.Gaps = append(r.Gaps, Gap{From: a.T, To: b.T})
			continue
		}
		r.Joules += (a.V + b.V) / 2 * dt.Seconds()
	}
	return r, nil
}

// FromCounter takes energy from a cumulative joule counter. The counter
// difference is exact even across a gap, so gaps are reported but the energy
// still counts: only its distribution over time inside the gap is unknown.
func FromCounter(points []Point, maxGap time.Duration) (Result, error) {
	var r Result
	if err := checkTimes(points); err != nil {
		return r, err
	}
	if len(points) < 2 {
		return r, nil
	}
	r.Span = points[len(points)-1].T.Sub(points[0].T)
	for i := 1; i < len(points); i++ {
		a, b := points[i-1], points[i]
		if b.V < a.V {
			return Result{}, fmt.Errorf("%w at %s", ErrCounterDecreased, b.T.Format(time.RFC3339Nano))
		}
		if b.T.Sub(a.T) > maxGap {
			r.Gaps = append(r.Gaps, Gap{From: a.T, To: b.T})
		}
	}
	r.Joules = points[len(points)-1].V - points[0].V
	return r, nil
}

func checkTimes(points []Point) error {
	for i := 1; i < len(points); i++ {
		if !points[i].T.After(points[i-1].T) {
			return fmt.Errorf("%w at index %d", ErrNotIncreasing, i)
		}
	}
	return nil
}
