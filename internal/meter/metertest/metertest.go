// Package metertest checks that a meter.Driver keeps the promises of the
// meter package. Every driver's tests run Contract against the driver's mock.
package metertest

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/wattproof/wattproof/internal/meter"
)

// Contract reads d the given number of times and fails t if any reading
// breaks the interface's rules: the driver's class on every reading, a channel
// and a sample time on every reading, finite non-negative power, the same
// channels on every read, and per channel neither time nor energy counter
// going backwards.
func Contract(t testing.TB, d meter.Driver, reads int) {
	t.Helper()
	class := d.Class()
	if class < meter.ClassA || class > meter.ClassD {
		t.Fatalf("Class() = %v, not one of A–D", class)
	}
	var channels map[string]bool
	lastTime := map[string]time.Time{}
	lastJoules := map[string]float64{}
	for i := range reads {
		rs, err := d.Read(context.Background())
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if len(rs) == 0 {
			t.Fatalf("read %d: no readings", i)
		}
		seen := map[string]bool{}
		for _, r := range rs {
			switch {
			case r.Channel == "":
				t.Errorf("read %d: reading without a channel", i)
			case seen[r.Channel]:
				t.Errorf("read %d: channel %q twice", i, r.Channel)
			}
			seen[r.Channel] = true
			if r.Class != class {
				t.Errorf("read %d, %s: class %v, driver's class %v", i, r.Channel, r.Class, class)
			}
			if r.Time.IsZero() {
				t.Errorf("read %d, %s: no sample time", i, r.Channel)
			}
			if math.IsNaN(r.Watts) || math.IsInf(r.Watts, 0) || r.Watts < 0 {
				t.Errorf("read %d, %s: power %v W", i, r.Channel, r.Watts)
			}
			if prev, ok := lastTime[r.Channel]; ok && r.Time.Before(prev) {
				t.Errorf("read %d, %s: time %v before the previous %v", i, r.Channel, r.Time, prev)
			}
			lastTime[r.Channel] = r.Time
			if r.HasJoules {
				if math.IsNaN(r.Joules) || math.IsInf(r.Joules, 0) || r.Joules < 0 {
					t.Errorf("read %d, %s: energy counter %v J", i, r.Channel, r.Joules)
				}
				if prev, ok := lastJoules[r.Channel]; ok && r.Joules < prev {
					t.Errorf("read %d, %s: energy counter fell from %v to %v J", i, r.Channel, prev, r.Joules)
				}
				lastJoules[r.Channel] = r.Joules
			}
		}
		if channels == nil {
			channels = seen
		} else if len(seen) != len(channels) {
			t.Errorf("read %d: %d channels, first read had %d", i, len(seen), len(channels))
		} else {
			for c := range seen {
				if !channels[c] {
					t.Errorf("read %d: new channel %q", i, c)
				}
			}
		}
	}
}
