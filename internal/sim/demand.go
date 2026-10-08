package sim

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
)

// Forecaster predicts requested CPU.
type Forecaster interface {
	// Forecast returns the requested CPU h steps after the last step of
	// history.
	Forecast(history []float64, h int) float64
}

// SeasonalNaive forecasts the value one period earlier, scaled by how today
// compares with the same time one period ago. Before a full period of history
// exists, it repeats the latest value.
type SeasonalNaive struct {
	// Period is the season length in steps, for example one day.
	Period int
}

func (f SeasonalNaive) Forecast(history []float64, h int) float64 {
	now := len(history) - 1
	if now < f.Period || h > f.Period {
		return history[now]
	}
	last := history[now-f.Period]
	level := 1.0
	if last > 0 {
		level = min(max(history[now]/last, 0.5), 2)
	}
	return history[now-f.Period+h] * level
}

// Oracle knows the future. It bounds what any forecast could achieve and is
// labelled as such wherever it is reported.
type Oracle struct {
	Series []float64
}

func (f Oracle) Forecast(history []float64, h int) float64 {
	return f.Series[min(len(history)-1+h, len(f.Series)-1)]
}

// ReadShape reads a demand curve from CSV: a header line, then lines whose
// last field is a number. Lines starting with '#' are comments. The values
// are scaled so that the largest is 1.
func ReadShape(r io.Reader) ([]float64, error) {
	var v []float64
	header := false
	sc := bufio.NewScanner(r)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if !header {
			header = true
			continue
		}
		f := strings.Split(text, ",")
		x, err := strconv.ParseFloat(strings.TrimSpace(f[len(f)-1]), 64)
		if err != nil || x < 0 {
			return nil, fmt.Errorf("line %d: bad value %q", line, f[len(f)-1])
		}
		v = append(v, x)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(v) == 0 || slices.Max(v) == 0 {
		return nil, fmt.Errorf("no positive values")
	}
	peak := slices.Max(v)
	for i := range v {
		v[i] /= peak
	}
	return v, nil
}

// Interpolate turns one value per period into perPeriod values per period,
// linearly, wrapping from the last value back to the first.
func Interpolate(v []float64, perPeriod int) []float64 {
	out := make([]float64, 0, len(v)*perPeriod)
	for i, a := range v {
		b := v[(i+1)%len(v)]
		for k := range perPeriod {
			out = append(out, a+(b-a)*float64(k)/float64(perPeriod))
		}
	}
	return out
}
