// Package redfish reads server power from a BMC over Redfish and switches the
// server on and off. Its readings are class C (docs/measurement.md): good
// enough to run on, never for headline claims.
//
// Per chassis, power is read from the first source that has it:
//
//  1. the sensor behind EnvironmentMetrics.PowerWatts, if it states when it
//     took the reading (ReadingTime);
//  2. EnvironmentMetrics.PowerWatts;
//  3. the input power of the installed power supplies (PowerSubsystem), summed;
//  4. the deprecated Power resource.
//
// Only the first gives the device's sample time. The others are timed when the
// response arrives, which the inventory records as a quirk. Everything the
// driver finds odd about a BMC goes into Inventory.Quirks, so that over time
// the quirks of each vendor, model and firmware can be published.
package redfish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/wattproof/wattproof/internal/meter"
)

// Config says how to reach one BMC.
type Config struct {
	// Endpoint is the BMC's base URL, e.g. https://10.0.0.5.
	Endpoint string
	// Username and Password are sent with HTTP basic authentication.
	Username, Password string
	// Client makes the requests; nil means http.DefaultClient. BMCs often
	// use self-signed certificates: trust them in the client's TLS config.
	Client *http.Client
}

// Source is where a chassis's power reading comes from.
type Source string

const (
	SourceSensor             Source = "Sensor"
	SourceEnvironmentMetrics Source = "EnvironmentMetrics"
	SourcePowerSubsystem     Source = "PowerSubsystem"
	SourcePower              Source = "Power (deprecated)"
)

// Chassis is one chassis that reports power. Its ID is the channel of its
// readings.
type Chassis struct {
	ID                  string
	Manufacturer, Model string
	Source              Source
	// Accuracy is the ± percent error the sensor states, 0 if it states none.
	Accuracy float64
	// SensingInterval is the sensor's stated interval between readings, as
	// an ISO 8601 duration, empty if it states none.
	SensingInterval string
}

// Inventory is what the driver learnt about the BMC when it connected.
type Inventory struct {
	RedfishVersion string
	Vendor         string
	// Firmware lists each manager (BMC) as "manufacturer model firmware".
	Firmware []string
	Chassis  []Chassis
	// Systems lists the computer systems that offer ComputerSystem.Reset.
	Systems []string
	Quirks  []string
}

// plan is how to read one chassis.
type plan struct {
	Chassis
	sensor, env string   // resource paths
	envEnergy   bool     // EnvironmentMetrics carries an energy counter
	psus        []string // PowerSupplyMetrics paths
	power       string   // deprecated Power path
}

// Driver reads one BMC. It is safe for concurrent use.
type Driver struct {
	c     client
	now   func() time.Time
	plans []*plan

	mu    sync.Mutex
	inv   Inventory
	last  map[string]time.Time
	noted map[string]bool
}

// New connects to the BMC, finds every chassis that reports power and how to
// read it, and the systems it can reset.
func New(ctx context.Context, cfg Config) (*Driver, error) {
	base, err := url.Parse(strings.TrimRight(cfg.Endpoint, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("redfish: endpoint %q is not a URL", cfg.Endpoint)
	}
	hc := cfg.Client
	if hc == nil {
		hc = http.DefaultClient
	}
	d := &Driver{
		c:     client{base: base, user: cfg.Username, pass: cfg.Password, hc: hc},
		now:   time.Now,
		last:  map[string]time.Time{},
		noted: map[string]bool{},
	}
	if err := d.discover(ctx); err != nil {
		return nil, err
	}
	return d, nil
}

// Class is C: a BMC.
func (d *Driver) Class() meter.Class { return meter.ClassC }

// Inventory returns what the driver has learnt about the BMC, including the
// quirks met while reading it.
func (d *Driver) Inventory() Inventory {
	d.mu.Lock()
	defer d.mu.Unlock()
	inv := d.inv
	inv.Firmware = slices.Clone(inv.Firmware)
	inv.Chassis = slices.Clone(inv.Chassis)
	inv.Systems = slices.Clone(inv.Systems)
	inv.Quirks = slices.Clone(inv.Quirks)
	return inv
}

func (d *Driver) quirk(format string, args ...any) {
	q := fmt.Sprintf(format, args...)
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.noted[q] {
		d.noted[q] = true
		d.inv.Quirks = append(d.inv.Quirks, q)
	}
}

// Redfish resources, only the properties the driver uses.
type (
	link struct {
		ID string `json:"@odata.id"`
	}
	collection struct{ Members []link }
	excerpt    struct {
		Reading       *float64
		DataSourceURI string `json:"DataSourceUri"`
	}
	serviceRoot struct {
		RedfishVersion             string
		Vendor                     string
		Chassis, Systems, Managers *link
	}
	manager    struct{ Manufacturer, Model, FirmwareVersion string }
	chassisRes struct {
		ID                                        string `json:"@odata.id"`
		Manufacturer, Model                       string
		EnvironmentMetrics, PowerSubsystem, Power *link
	}
	envMetrics struct{ PowerWatts, EnergyJoules, EnergykWh *excerpt }
	sensorRes  struct {
		Reading         *float64
		ReadingUnits    string
		ReadingTime     string
		Accuracy        *float64
		SensingInterval string
	}
	powerSubsystem struct{ PowerSupplies *link }
	powerSupply    struct {
		ID      string `json:"@odata.id"`
		Status  struct{ State string }
		Metrics *link
	}
	psuMetrics struct{ InputPowerWatts, EnergykWh *excerpt }
	powerRes   struct {
		PowerControl []struct{ PowerConsumedWatts *float64 }
	}
)

func (d *Driver) discover(ctx context.Context) error {
	var root serviceRoot
	if err := d.c.get(ctx, "/redfish/v1/", &root); err != nil {
		return err
	}
	d.inv.RedfishVersion, d.inv.Vendor = root.RedfishVersion, root.Vendor
	if root.Managers != nil {
		var ms collection
		if err := d.c.get(ctx, root.Managers.ID, &ms); err != nil {
			return err
		}
		for _, l := range ms.Members {
			var m manager
			if err := d.c.get(ctx, l.ID, &m); err != nil {
				return err
			}
			if f := strings.Join(strings.Fields(m.Manufacturer+" "+m.Model+" "+m.FirmwareVersion), " "); f != "" {
				d.inv.Firmware = append(d.inv.Firmware, f)
			}
		}
	}
	if root.Chassis == nil {
		return errors.New("redfish: the service has no Chassis collection")
	}
	var cs collection
	if err := d.c.get(ctx, root.Chassis.ID, &cs); err != nil {
		return err
	}
	for _, l := range cs.Members {
		var ch chassisRes
		if err := d.c.get(ctx, l.ID, &ch); err != nil {
			return err
		}
		if ch.ID == "" {
			ch.ID = l.ID
		}
		p, err := d.plan(ctx, ch)
		if err != nil {
			return err
		}
		if p.Source != "" {
			d.plans = append(d.plans, p)
			d.inv.Chassis = append(d.inv.Chassis, p.Chassis)
		}
	}
	if len(d.plans) == 0 {
		return errors.New("redfish: no chassis reports power")
	}
	if root.Systems != nil {
		var ss collection
		if err := d.c.get(ctx, root.Systems.ID, &ss); err != nil {
			return err
		}
		for _, l := range ss.Members {
			var s systemRes
			if err := d.c.get(ctx, l.ID, &s); err != nil {
				return err
			}
			if s.Actions.Reset != nil && s.Actions.Reset.Target != "" {
				d.inv.Systems = append(d.inv.Systems, l.ID)
			}
		}
	}
	return nil
}

// plan finds how to read one chassis. A chassis without any power data gets a
// plan with no source.
func (d *Driver) plan(ctx context.Context, ch chassisRes) (*plan, error) {
	p := &plan{Chassis: Chassis{ID: ch.ID, Manufacturer: ch.Manufacturer, Model: ch.Model}}
	var envW float64
	if ch.EnvironmentMetrics != nil {
		var em envMetrics
		if err := d.c.get(ctx, ch.EnvironmentMetrics.ID, &em); err != nil {
			return nil, err
		}
		if em.PowerWatts != nil && em.PowerWatts.Reading != nil {
			envW = *em.PowerWatts.Reading
			p.Source, p.env = SourceEnvironmentMetrics, ch.EnvironmentMetrics.ID
			p.envEnergy = hasReading(em.EnergyJoules) || hasReading(em.EnergykWh)
			if src := em.PowerWatts.DataSourceURI; src != "" {
				var s sensorRes
				if err := d.c.get(ctx, src, &s); err != nil {
					d.quirk("%s: power sensor %s unreadable: %v", ch.ID, src, err)
				} else {
					if s.ReadingUnits != "" && s.ReadingUnits != "W" {
						return nil, fmt.Errorf("redfish: %s: power sensor reads in %q, not W", ch.ID, s.ReadingUnits)
					}
					if s.Accuracy != nil {
						p.Accuracy = *s.Accuracy
					}
					p.SensingInterval = s.SensingInterval
					if _, err := time.Parse(time.RFC3339, s.ReadingTime); err == nil && s.Reading != nil {
						p.Source, p.sensor = SourceSensor, src
					}
				}
			}
		} else {
			d.quirk("%s: EnvironmentMetrics has no PowerWatts reading", ch.ID)
		}
	}
	var psuW float64
	if ch.PowerSubsystem != nil {
		var err error
		if p.psus, psuW, err = d.powerSupplies(ctx, ch); err != nil {
			return nil, err
		}
		if p.Source == "" && len(p.psus) > 0 {
			p.Source = SourcePowerSubsystem
		}
	}
	if p.env != "" && len(p.psus) > 0 && math.Abs(envW-psuW) > 0.1*math.Max(envW, psuW) {
		d.quirk("%s: chassis power %g W, but its power supplies' input sums to %g W", ch.ID, envW, psuW)
	}
	if p.Source == "" && ch.Power != nil {
		var pw powerRes
		if err := d.c.get(ctx, ch.Power.ID, &pw); err != nil {
			return nil, err
		}
		if len(pw.PowerControl) > 0 && pw.PowerControl[0].PowerConsumedWatts != nil {
			p.Source, p.power = SourcePower, ch.Power.ID
			d.quirk("%s: power only in the deprecated Power resource", ch.ID)
		}
	}
	if p.Source != "" && p.Source != SourceSensor {
		d.quirk("%s: no sensor sample time; readings are timed on arrival", ch.ID)
	}
	return p, nil
}

// powerSupplies returns the metrics paths of the installed power supplies that
// report input power, and that power summed.
func (d *Driver) powerSupplies(ctx context.Context, ch chassisRes) ([]string, float64, error) {
	var ps powerSubsystem
	if err := d.c.get(ctx, ch.PowerSubsystem.ID, &ps); err != nil {
		return nil, 0, err
	}
	if ps.PowerSupplies == nil {
		return nil, 0, nil
	}
	var members collection
	if err := d.c.get(ctx, ps.PowerSupplies.ID, &members); err != nil {
		return nil, 0, err
	}
	var paths []string
	var sum float64
	for _, l := range members.Members {
		var psu powerSupply
		if err := d.c.get(ctx, l.ID, &psu); err != nil {
			return nil, 0, err
		}
		if psu.Status.State == "Absent" {
			continue
		}
		if psu.Metrics == nil {
			d.quirk("%s: power supply %s has no metrics", ch.ID, l.ID)
			continue
		}
		var m psuMetrics
		if err := d.c.get(ctx, psu.Metrics.ID, &m); err != nil {
			return nil, 0, err
		}
		if !hasReading(m.InputPowerWatts) {
			d.quirk("%s: power supply %s reports no input power", ch.ID, l.ID)
			continue
		}
		paths = append(paths, psu.Metrics.ID)
		sum += *m.InputPowerWatts.Reading
	}
	return paths, sum, nil
}

func hasReading(e *excerpt) bool { return e != nil && e.Reading != nil }

// Read returns one reading per chassis that reports power.
func (d *Driver) Read(ctx context.Context) ([]meter.Reading, error) {
	out := make([]meter.Reading, 0, len(d.plans))
	for _, p := range d.plans {
		r, err := d.read(ctx, p)
		if err != nil {
			return nil, fmt.Errorf("redfish: %s: %w", p.ID, err)
		}
		out = append(out, r)
	}
	return out, nil
}

func (d *Driver) read(ctx context.Context, p *plan) (meter.Reading, error) {
	r := meter.Reading{Channel: p.ID, Class: meter.ClassC}
	switch p.Source {
	case SourceSensor:
		var s sensorRes
		if err := d.c.get(ctx, p.sensor, &s); err != nil {
			return r, err
		}
		t, err := time.Parse(time.RFC3339, s.ReadingTime)
		if err != nil || s.Reading == nil {
			return r, fmt.Errorf("sensor %s gave no reading or no time", p.sensor)
		}
		r.Watts, r.Time = *s.Reading, t
		d.mu.Lock()
		stale := d.last[p.ID].Equal(t)
		d.last[p.ID] = t
		d.mu.Unlock()
		if stale {
			d.quirk("%s: the sensor repeated a reading (same ReadingTime on consecutive reads)", p.ID)
		}
		if p.envEnergy {
			var em envMetrics
			if err := d.c.get(ctx, p.env, &em); err != nil {
				return r, err
			}
			d.energy(p.ID, &r, em)
		}
	case SourceEnvironmentMetrics:
		var em envMetrics
		if err := d.c.get(ctx, p.env, &em); err != nil {
			return r, err
		}
		if !hasReading(em.PowerWatts) {
			return r, errors.New("no PowerWatts reading in EnvironmentMetrics")
		}
		r.Watts, r.Time = *em.PowerWatts.Reading, d.now()
		d.energy(p.ID, &r, em)
	case SourcePowerSubsystem:
		var kWh float64
		all := true
		for _, path := range p.psus {
			var m psuMetrics
			if err := d.c.get(ctx, path, &m); err != nil {
				return r, err
			}
			if !hasReading(m.InputPowerWatts) {
				return r, fmt.Errorf("power supply %s gave no input power", path)
			}
			r.Watts += *m.InputPowerWatts.Reading
			if hasReading(m.EnergykWh) {
				kWh += *m.EnergykWh.Reading
				d.wholeKWh(p.ID, *m.EnergykWh.Reading)
			} else {
				all = false
			}
		}
		r.Time = d.now()
		if all {
			r.Joules, r.HasJoules = kWh*3.6e6, true
		}
	case SourcePower:
		var pw powerRes
		if err := d.c.get(ctx, p.power, &pw); err != nil {
			return r, err
		}
		if len(pw.PowerControl) == 0 || pw.PowerControl[0].PowerConsumedWatts == nil {
			return r, errors.New("no PowerConsumedWatts in the Power resource")
		}
		r.Watts, r.Time = *pw.PowerControl[0].PowerConsumedWatts, d.now()
	}
	return r, nil
}

// energy sets the reading's counter from EnvironmentMetrics, in joules when
// the BMC gives joules.
func (d *Driver) energy(ch string, r *meter.Reading, em envMetrics) {
	switch {
	case hasReading(em.EnergyJoules):
		r.Joules, r.HasJoules = *em.EnergyJoules.Reading, true
	case hasReading(em.EnergykWh):
		r.Joules, r.HasJoules = *em.EnergykWh.Reading*3.6e6, true
		d.wholeKWh(ch, *em.EnergykWh.Reading)
	}
}

func (d *Driver) wholeKWh(ch string, kWh float64) {
	if kWh == math.Trunc(kWh) {
		d.quirk("%s: energy counter in whole kWh (steps of 3.6 MJ)", ch)
	}
}

// client sends requests to one BMC and never follows a link off it.
type client struct {
	base       *url.URL
	user, pass string
	hc         *http.Client
}

func (c *client) url(path string) (string, error) {
	ref, err := url.Parse(path)
	if err != nil {
		return "", fmt.Errorf("redfish: link %q: %w", path, err)
	}
	u := c.base.ResolveReference(ref)
	if u.Scheme != c.base.Scheme || u.Host != c.base.Host {
		return "", fmt.Errorf("redfish: link %q leaves %s", path, c.base.Host)
	}
	return u.String(), nil
}

func (c *client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	u, err := c.url(path)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("OData-Version", "4.0")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("redfish: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("redfish: %s %s: %w", method, path, err)
	}
	if resp.StatusCode/100 != 2 {
		msg := strings.Join(strings.Fields(string(data)), " ")
		if len(msg) > 200 {
			msg = msg[:200] + "…"
		}
		return fmt.Errorf("redfish: %s %s: %s %s", method, path, resp.Status, msg)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("redfish: %s %s: %w", method, path, err)
	}
	return nil
}
