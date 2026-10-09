// Command wattproof-traceprep turns per-service hourly rows from the Google
// 2019 trace (analysis/traces/google2019_cell.sql) into hourly totals per group
// of services, with the requests each simulator arm would set. Rightsizing and
// VPA learn only from earlier hours, so the totals can be scored out of sample
// (docs/research/trace-analysis-plan.md).
//
//	go run ./cmd/wattproof-traceprep -in cell_a.csv -out testdata/google2019-cella-services.csv
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"

	"github.com/wattproof/wattproof/internal/sim"
)

const (
	hours  = 744 // May 2019, from 00:00 PT on 1 May
	day    = 24
	week   = 7 * day
	margin = 0.2  // shaped request = learned hourly peak + 20% (ADR-0010)
	floor  = 0.1  // never below 10% of the template
	vpaQ   = 0.9  // VPA: p90 of usage ...
	vpaM   = 0.15 // ... plus 15%, over 8 days, updated daily
	memM   = 0.15 // memory set to the learned weekly peak + 15% (arm T+R+M)
)

type service struct {
	use, peak5, req, memUse, memReq [hours]float64
	present                         [hours]bool
	manual                          bool
}

func main() {
	in := flag.String("in", "", "CSV of the trace query's result")
	out := flag.String("out", "", "CSV to write")
	flag.Parse()
	if err := run(*in, *out); err != nil {
		fmt.Fprintln(os.Stderr, "wattproof-traceprep:", err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	svcs, err := read(bufio.NewReaderSize(f, 1<<20))
	if err != nil {
		return err
	}
	groups := map[string]func(s *service, daily bool) bool{
		"all":          func(*service, bool) bool { return true },
		"manual":       func(s *service, _ bool) bool { return s.manual },
		"daily":        func(_ *service, d bool) bool { return d },
		"manual-daily": func(s *service, d bool) bool { return s.manual && d },
	}
	order := []string{"all", "manual", "daily", "manual-daily"}
	cols := []string{"cpu_use", "cpu_req", "cpu_shaped", "cpu_vpa", "mem_use", "mem_req", "mem_vpa", "mem_peak", "mem_short_vpa", "mem_short_peak"}
	sums := map[string][][]float64{}
	counts := map[string]int{}
	for _, g := range order {
		sums[g] = make([][]float64, len(cols))
		for c := range cols {
			sums[g][c] = make([]float64, hours)
		}
	}
	for _, s := range svcs {
		daily := dailyCycle(s.use[:], s.present[:])
		series := arms(s)
		for _, g := range order {
			if !groups[g](s, daily) {
				continue
			}
			counts[g]++
			for c := range cols {
				for h := range hours {
					sums[g][c][h] += series[c][h]
				}
			}
		}
	}

	w, err := os.Create(out)
	if err != nil {
		return err
	}
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "# Hourly totals of long-running services, Google cluster trace 2019, cell a (May 2019).")
	fmt.Fprintln(bw, "# Source: Google, CC-BY 4.0, https://github.com/google/cluster-data. Derived by")
	fmt.Fprintln(bw, "# analysis/traces/google2019_cell.sql and cmd/wattproof-traceprep. Units: Google's")
	fmt.Fprintln(bw, "# normalised CPU and memory. Hour 0 starts 2019-05-01 00:00 PT.")
	fmt.Fprintln(bw, "# cpu_shaped and mem_peak learn from earlier weeks only; cpu_vpa and mem_vpa from the")
	fmt.Fprintln(bw, "# trailing 8 days, updated daily. mem_short_*: memory usage above that request.")
	for _, g := range order {
		fmt.Fprintf(bw, "# services in %s: %d\n", g, counts[g])
	}
	fmt.Fprint(bw, "hour,group")
	for _, c := range cols {
		fmt.Fprint(bw, ",", c)
	}
	fmt.Fprintln(bw)
	for _, g := range order {
		for h := range hours {
			fmt.Fprintf(bw, "%d,%s", h, g)
			for c := range cols {
				fmt.Fprintf(bw, ",%.6g", sums[g][c][h])
			}
			fmt.Fprintln(bw)
		}
	}
	if err := bw.Flush(); err != nil {
		return err
	}
	return w.Close()
}

// arms returns one service's hourly series, in the order of run's columns.
// Hours in which the service did not run are zero in every series.
func arms(s *service) [][]float64 {
	use, peak, req := s.use[:], s.peak5[:], s.req[:]
	floors := make([]float64, hours)
	for h := range floors {
		floors[h] = floor * req[h]
	}
	shaped := sim.ShapeLearned(peak, req, floors, 1, week, margin)
	vpa := sim.VPALearned(use, req, day, 8*day, vpaQ, vpaM)
	memVPA := sim.VPALearned(s.memUse[:], s.memReq[:], day, 8*day, vpaQ, vpaM)
	memPeak := sim.ShapeLearned(s.memUse[:], s.memReq[:], make([]float64, hours), week, week, memM)
	short := func(r []float64) []float64 {
		o := make([]float64, hours)
		for h := range o {
			o[h] = max(0, s.memUse[h]-r[h])
		}
		return o
	}
	out := [][]float64{slices.Clone(use), slices.Clone(req), shaped, vpa,
		slices.Clone(s.memUse[:]), slices.Clone(s.memReq[:]), memVPA, memPeak, short(memVPA), short(memPeak)}
	for _, series := range out {
		for h := range series {
			if !s.present[h] {
				series[h] = 0
			}
		}
	}
	return out
}

// dailyCycle is the plan's test: on at least 70% of full days, the day's 24
// hourly means correlate at 0.8 or more with the median day, and the median
// day swings at least 20% of its peak. At least 7 full days are needed.
func dailyCycle(use []float64, present []bool) bool {
	var days [][]float64
	for d := 0; d+day <= hours; d += day {
		full := true
		for h := d; h < d+day; h++ {
			full = full && present[h]
		}
		if full {
			days = append(days, use[d:d+day])
		}
	}
	if len(days) < 7 {
		return false
	}
	median := make([]float64, day)
	col := make([]float64, len(days))
	for h := range day {
		for i, d := range days {
			col[i] = d[h]
		}
		slices.Sort(col)
		n := len(col)
		median[h] = (col[(n-1)/2] + col[n/2]) / 2
	}
	top, bottom := slices.Max(median), slices.Min(median)
	if top <= 0 || (top-bottom)/top < 0.2 {
		return false
	}
	good := 0
	for _, d := range days {
		if pearson(d, median) >= 0.8 {
			good++
		}
	}
	return float64(good) >= 0.7*float64(len(days))
}

func pearson(a, b []float64) float64 {
	var ma, mb float64
	for i := range a {
		ma += a[i]
		mb += b[i]
	}
	ma /= float64(len(a))
	mb /= float64(len(b))
	var sab, saa, sbb float64
	for i := range a {
		x, y := a[i]-ma, b[i]-mb
		sab += x * y
		saa += x * x
		sbb += y * y
	}
	if saa == 0 || sbb == 0 {
		return 0
	}
	return sab / math.Sqrt(saa*sbb)
}

// read parses the query's rows and keeps those of kind "svc".
func read(r io.Reader) (map[string]*service, error) {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	col := map[string]int{}
	for i, h := range header {
		col[h] = i
	}
	for _, need := range []string{"kind", "k", "t", "cpu_use", "cpu_use_max5", "cpu_req", "mem_use", "mem_req", "vscale"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("missing column %q", need)
		}
	}
	num := func(rec []string, name string) float64 {
		v, err := strconv.ParseFloat(rec[col[name]], 64)
		if err != nil {
			return 0
		}
		return v
	}
	svcs := map[string]*service{}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return svcs, nil
		}
		if err != nil {
			return nil, err
		}
		if rec[col["kind"]] != "svc" {
			continue
		}
		t, err := strconv.Atoi(rec[col["t"]])
		if err != nil || t < 0 || t >= hours {
			continue
		}
		s := svcs[rec[col["k"]]]
		if s == nil {
			s = &service{manual: rec[col["vscale"]] == "1"}
			svcs[rec[col["k"]]] = s
		}
		s.present[t] = true
		s.use[t] = num(rec, "cpu_use")
		s.peak5[t] = num(rec, "cpu_use_max5")
		s.req[t] = num(rec, "cpu_req")
		s.memUse[t] = num(rec, "mem_use")
		s.memReq[t] = num(rec, "mem_req")
	}
}
