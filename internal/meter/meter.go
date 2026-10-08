// Package meter defines how Wattproof reads power from measurement devices.
//
// Every reading carries the class of the device that produced it, so that no
// number downstream can lose track of how much it can be trusted. The classes
// are defined in docs/measurement.md.
package meter

import (
	"context"
	"fmt"
	"time"
)

// Class ranks a measurement source by accuracy. Lower is better.
type Class int

const (
	// ClassA is a reference power analyser, about ±0.1–0.2%.
	ClassA Class = iota + 1
	// ClassB is an outlet-metered PDU, about ±1%.
	ClassB
	// ClassC is a server's BMC (Redfish, IPMI DCMI). Coarse and slow.
	ClassC
	// ClassD is a component estimate such as RAPL or NVML. Never a total.
	ClassD
)

func (c Class) String() string {
	switch c {
	case ClassA:
		return "A"
	case ClassB:
		return "B"
	case ClassC:
		return "C"
	case ClassD:
		return "D"
	default:
		return fmt.Sprintf("Class(%d)", int(c))
	}
}

// Headline reports whether numbers of this class may be used for published
// savings claims (ADR-0001).
func (c Class) Headline() bool {
	return c == ClassA || c == ClassB
}

// Reading is one sample from one measured point, usually a power outlet.
type Reading struct {
	// Channel identifies the measured point, e.g. a PDU outlet or a server.
	Channel string
	// Time is when the device took the sample, not when it was received.
	Time time.Time
	// Watts is the active power at Time.
	Watts float64
	// Joules is the device's cumulative energy counter, if it has one.
	// Counters are preferred over integrating Watts.
	Joules float64
	// HasJoules is false when the device does not report a counter.
	HasJoules bool
	// Class is the accuracy class of the device.
	Class Class
}

// Driver reads one measurement device. Implementations exist per device
// family: reference analysers (SCPI), PDUs (SNMP, HTTP) and BMCs (Redfish,
// IPMI).
type Driver interface {
	// Class is the accuracy class of every reading this driver returns.
	Class() Class
	// Read returns the current reading of every channel the device exposes.
	Read(ctx context.Context) ([]Reading, error)
}
