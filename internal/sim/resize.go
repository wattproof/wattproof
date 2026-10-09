package sim

import (
	"slices"
)

// ShapeRequests returns CPU requests that follow usage through the day, as
// the resize controller does (ADR-0010): in each period (an hour of the week),
// the highest usage seen in that period plus a margin, never below floor and
// never above the template's request orig. Requests change at period
// boundaries, so within a period they always cover usage plus the margin.
//
// The highest usage per period stands in for the high quantile the
// controller learns from past weeks; with a replayed week it is exact.
func ShapeRequests(usage []float64, period int, orig, margin, floor float64) []float64 {
	out := make([]float64, len(usage))
	for start := 0; start < len(usage); start += period {
		end := min(start+period, len(usage))
		target := min(orig, max(floor, slices.Max(usage[start:end])*(1+margin)))
		for i := start; i < end; i++ {
			out[i] = target
		}
	}
	return out
}

// VPATarget returns the constant CPU request the Vertical Pod Autoscaler's
// default recommender converges to: the q quantile of usage over its window
// (8 days by default, so a week here) plus a margin (15% by default). It
// follows usage over days, not the hour of day.
func VPATarget(usage []float64, q, margin float64) float64 {
	s := slices.Clone(usage)
	slices.Sort(s)
	i := min(int(q*float64(len(s))), len(s)-1)
	return s[i] * (1 + margin)
}

// ShapeLearned is ShapeRequests learned from the past only, as the resize
// controller runs: the request in a period is the highest usage seen in the
// same period of earlier seasons (the same hour of earlier weeks) plus the
// margin, within [floor, orig]. In the first season nothing is learned yet and
// the template's request orig holds. orig and floor are per step, because a
// template can change. peak is the usage to learn from, for example the
// highest 5 minutes of each hour.
func ShapeLearned(peak, orig, floor []float64, period, season int, margin float64) []float64 {
	out := make([]float64, len(peak))
	learned := make([]float64, season/period+1) // highest peak per period of the season
	seen := make([]bool, len(learned))
	for start := 0; start < len(peak); start += period {
		end := min(start+period, len(peak))
		p := (start % season) / period
		for i := start; i < end; i++ {
			out[i] = orig[i]
			if seen[p] {
				out[i] = min(orig[i], max(floor[i], learned[p]*(1+margin)))
			}
		}
		// Learn after use: this period's usage informs the next season only.
		learned[p] = max(learned[p], slices.Max(peak[start:end]))
		seen[p] = true
	}
	return out
}

// VPALearned is the Vertical Pod Autoscaler's request over time: every
// update steps, the q quantile of usage over the trailing window plus the
// margin. Before the first update the template's request orig holds. Unlike
// shaping, VPA may raise a request above the template.
func VPALearned(usage, orig []float64, update, window int, q, margin float64) []float64 {
	out := make([]float64, len(usage))
	target := -1.0
	for i := range usage {
		if i > 0 && i%update == 0 {
			target = VPATarget(usage[max(0, i-window):i], q, margin)
		}
		out[i] = orig[i]
		if target >= 0 {
			out[i] = target
		}
	}
	return out
}
