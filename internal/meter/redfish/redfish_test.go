package redfish

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wattproof/wattproof/internal/meter"
	"github.com/wattproof/wattproof/internal/meter/metertest"
)

const (
	chassis1U = "/redfish/v1/Chassis/1U"
	system    = "/redfish/v1/Systems/437XR1138R2"
	sensor    = "/redfish/v1/Chassis/1U/Sensors/TotalPower"
	envPath   = "/redfish/v1/Chassis/1U/EnvironmentMetrics"
)

// mock serves DMTF's public-rackmount1 mockup as DMTF's mockup server does,
// with per-test edits. An edit that returns nil makes the resource 404.
type mock struct {
	t     *testing.T
	edits map[string]func(map[string]any) map[string]any
	user  string // required basic-auth user, if set

	mu    sync.Mutex
	posts []post
}

type post struct {
	path string
	body map[string]any
}

func newMock(t *testing.T) *mock {
	return &mock{t: t, edits: map[string]func(map[string]any) map[string]any{}}
}

func (m *mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if m.user != "" {
		if u, p, ok := r.BasicAuth(); !ok || u != m.user || p != "secret" {
			http.Error(w, `{"error":{"code":"Base.1.0.NoValidSession"}}`, http.StatusUnauthorized)
			return
		}
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	if r.Method == http.MethodPost {
		var b map[string]any
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		m.mu.Lock()
		m.posts = append(m.posts, post{path, b})
		m.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(path, "/redfish/v1"), "/")
	var res map[string]any
	if data, err := os.ReadFile(filepath.Join("testdata", "public-rackmount1", rel, "index.json")); err == nil {
		if err := json.Unmarshal(data, &res); err != nil {
			m.t.Fatalf("mockup %s: %v", rel, err)
		}
	}
	if edit, ok := m.edits[path]; ok {
		res = edit(res)
	}
	if res == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func (m *mock) start(t *testing.T) (*Driver, error) {
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return New(context.Background(), Config{Endpoint: srv.URL, Username: m.user, Password: "secret", Client: srv.Client()})
}

func (m *mock) must(t *testing.T) *Driver {
	t.Helper()
	d, err := m.start(t)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func drop(keys ...string) func(map[string]any) map[string]any {
	return func(r map[string]any) map[string]any {
		for _, k := range keys {
			delete(r, k)
		}
		return r
	}
}

func hasQuirk(inv Inventory, sub string) bool {
	return slices.ContainsFunc(inv.Quirks, func(q string) bool { return strings.Contains(q, sub) })
}

func readOne(t *testing.T, d *Driver) meter.Reading {
	t.Helper()
	rs, err := d.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 {
		t.Fatalf("%d readings, want 1", len(rs))
	}
	return rs[0]
}

func TestContract(t *testing.T) {
	metertest.Contract(t, newMock(t).must(t), 3)
}

func TestSensorWithSampleTime(t *testing.T) {
	d := newMock(t).must(t)
	r := readOne(t, d)
	want := time.Date(2019, 8, 13, 4, 14, 33, 0, time.FixedZone("", 6*3600))
	if r.Channel != chassis1U || r.Watts != 374 || !r.Time.Equal(want) || r.Class != meter.ClassC || r.HasJoules {
		t.Errorf("reading %+v, want 374 W at %v on %s, class C, no counter", r, want, chassis1U)
	}
	inv := d.Inventory()
	if inv.RedfishVersion != "1.15.0" || !slices.Equal(inv.Firmware, []string{"Joo Janta 200 1.45.455b66-rev4"}) {
		t.Errorf("inventory %+v", inv)
	}
	c := inv.Chassis[0]
	if c.Source != SourceSensor || c.Manufacturer != "Contoso" || c.Model != "3500RX" || c.Accuracy != 0.2 || c.SensingInterval != "PT0.25S" {
		t.Errorf("chassis %+v", c)
	}
	if !slices.Equal(inv.Systems, []string{system}) {
		t.Errorf("systems %v", inv.Systems)
	}
	// The mockup's chassis reads 374 W and its power supply 937.4 W input.
	if !hasQuirk(inv, "power supplies' input sums to 937.4 W") {
		t.Errorf("disagreement not recorded: %q", inv.Quirks)
	}
	if hasQuirk(inv, "timed on arrival") {
		t.Errorf("sensor has a sample time, but quirks say otherwise: %q", inv.Quirks)
	}
}

func TestRepeatedReadingIsNoted(t *testing.T) {
	d := newMock(t).must(t)
	readOne(t, d)
	if hasQuirk(d.Inventory(), "repeated a reading") {
		t.Fatal("noted after one read")
	}
	readOne(t, d)
	if !hasQuirk(d.Inventory(), "repeated a reading") {
		t.Error("the mockup's fixed ReadingTime was not noted")
	}
}

func TestEnvironmentMetricsWithoutSampleTime(t *testing.T) {
	m := newMock(t)
	m.edits[sensor] = drop("ReadingTime")
	m.edits[envPath] = func(r map[string]any) map[string]any {
		r["EnergyJoules"] = map[string]any{"Reading": 1.5e9}
		return r
	}
	d := m.must(t)
	at := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	d.now = func() time.Time { return at }
	r := readOne(t, d)
	if r.Watts != 374 || !r.Time.Equal(at) || !r.HasJoules || r.Joules != 1.5e9 {
		t.Errorf("reading %+v, want 374 W, 1.5e9 J at %v", r, at)
	}
	inv := d.Inventory()
	if inv.Chassis[0].Source != SourceEnvironmentMetrics || !hasQuirk(inv, "timed on arrival") {
		t.Errorf("source %s, quirks %q", inv.Chassis[0].Source, inv.Quirks)
	}
}

func TestSensorWithSampleTimeReadsEnergy(t *testing.T) {
	m := newMock(t)
	m.edits[envPath] = func(r map[string]any) map[string]any {
		r["EnergykWh"] = map[string]any{"Reading": 12.5}
		return r
	}
	r := readOne(t, m.must(t))
	if !r.HasJoules || r.Joules != 12.5*3.6e6 {
		t.Errorf("energy %v (%v), want %v J", r.Joules, r.HasJoules, 12.5*3.6e6)
	}
}

func TestPowerSubsystemFallback(t *testing.T) {
	m := newMock(t)
	m.edits[chassis1U] = drop("EnvironmentMetrics")
	// An absent bay that still links metrics must not be counted.
	m.edits["/redfish/v1/Chassis/1U/PowerSubsystem/PowerSupplies/Bay2"] = func(r map[string]any) map[string]any {
		r["Metrics"] = map[string]any{"@odata.id": "/redfish/v1/Chassis/1U/PowerSubsystem/PowerSupplies/Bay1/Metrics"}
		return r
	}
	d := m.must(t)
	r := readOne(t, d)
	// Bay1 reports 937.4 W and 325675 kWh; Bay2 is absent.
	if r.Watts != 937.4 || !r.HasJoules || r.Joules != 325675*3.6e6 {
		t.Errorf("reading %+v, want 937.4 W and 325675 kWh", r)
	}
	inv := d.Inventory()
	if inv.Chassis[0].Source != SourcePowerSubsystem || !hasQuirk(inv, "whole kWh") || !hasQuirk(inv, "timed on arrival") {
		t.Errorf("source %s, quirks %q", inv.Chassis[0].Source, inv.Quirks)
	}
}

func TestDeprecatedPowerFallback(t *testing.T) {
	m := newMock(t)
	m.edits[chassis1U] = drop("EnvironmentMetrics", "PowerSubsystem")
	d := m.must(t)
	r := readOne(t, d)
	if r.Watts != 344 || r.HasJoules {
		t.Errorf("reading %+v, want 344 W from PowerControl, no counter", r)
	}
	inv := d.Inventory()
	if inv.Chassis[0].Source != SourcePower || !hasQuirk(inv, "deprecated Power") {
		t.Errorf("source %s, quirks %q", inv.Chassis[0].Source, inv.Quirks)
	}
}

func TestNoPowerData(t *testing.T) {
	m := newMock(t)
	m.edits[chassis1U] = drop("EnvironmentMetrics", "PowerSubsystem", "Power")
	if _, err := m.start(t); err == nil || !strings.Contains(err.Error(), "no chassis reports power") {
		t.Errorf("err = %v", err)
	}
}

func TestSensorInWrongUnit(t *testing.T) {
	m := newMock(t)
	m.edits[sensor] = func(r map[string]any) map[string]any {
		r["ReadingUnits"] = "kW"
		return r
	}
	if _, err := m.start(t); err == nil || !strings.Contains(err.Error(), `"kW"`) {
		t.Errorf("err = %v", err)
	}
}

func TestReadFailsLoudly(t *testing.T) {
	m := newMock(t)
	d := m.must(t)
	m.edits[sensor] = func(map[string]any) map[string]any { return nil }
	if _, err := d.Read(context.Background()); err == nil {
		t.Error("a sensor that disappears gave no error")
	}
}

func TestBasicAuth(t *testing.T) {
	m := newMock(t)
	m.user = "wattproof"
	m.must(t)

	m2 := newMock(t)
	m2.user = "wattproof"
	srv := httptest.NewServer(m2)
	defer srv.Close()
	_, err := New(context.Background(), Config{Endpoint: srv.URL, Username: "wattproof", Password: "wrong", Client: srv.Client()})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("wrong password: err = %v", err)
	}
	if strings.Contains(err.Error(), "wrong") {
		t.Errorf("error leaks the password: %v", err)
	}
}

func TestLinkOffTheBMCIsRefused(t *testing.T) {
	var offHost bool
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { offHost = true }))
	defer other.Close()
	m := newMock(t)
	m.edits["/redfish/v1/Chassis"] = func(r map[string]any) map[string]any {
		r["Members"] = []any{map[string]any{"@odata.id": other.URL + "/redfish/v1/Chassis/1U"}}
		return r
	}
	if _, err := m.start(t); err == nil || !strings.Contains(err.Error(), "leaves") {
		t.Errorf("err = %v", err)
	}
	if offHost {
		t.Error("a request went to another host")
	}
}

func TestReset(t *testing.T) {
	m := newMock(t)
	d := m.must(t)
	if err := d.Reset(context.Background(), system, GracefulShutdown); err != nil {
		t.Fatal(err)
	}
	want := post{system + "/Actions/ComputerSystem.Reset", map[string]any{"ResetType": "GracefulShutdown"}}
	if len(m.posts) != 1 || m.posts[0].path != want.path || m.posts[0].body["ResetType"] != "GracefulShutdown" {
		t.Errorf("posts %+v, want %+v", m.posts, want)
	}
	if s, err := d.PowerState(context.Background(), system); err != nil || s != "On" {
		t.Errorf("PowerState = %q, %v", s, err)
	}
}

func TestResetRefusesDisallowedType(t *testing.T) {
	m := newMock(t)
	m.edits[system] = func(r map[string]any) map[string]any {
		r["Actions"].(map[string]any)["#ComputerSystem.Reset"].(map[string]any)["ResetType@Redfish.AllowableValues"] = []any{"On", "ForceOff"}
		return r
	}
	d := m.must(t)
	if err := d.Reset(context.Background(), system, GracefulShutdown); err == nil || !strings.Contains(err.Error(), "does not allow") {
		t.Errorf("err = %v", err)
	}
	if len(m.posts) != 0 {
		t.Errorf("posted %+v", m.posts)
	}
}

func TestResetReadsActionInfo(t *testing.T) {
	m := newMock(t)
	info := "/redfish/v1/Systems/437XR1138R2/ResetActionInfo"
	m.edits[system] = func(r map[string]any) map[string]any {
		a := r["Actions"].(map[string]any)["#ComputerSystem.Reset"].(map[string]any)
		delete(a, "ResetType@Redfish.AllowableValues")
		a["@Redfish.ActionInfo"] = info
		return r
	}
	m.edits[info] = func(map[string]any) map[string]any {
		return map[string]any{"Parameters": []any{map[string]any{"Name": "ResetType", "AllowableValues": []any{"On"}}}}
	}
	d := m.must(t)
	if err := d.Reset(context.Background(), system, ForceOff); err == nil {
		t.Error("ForceOff allowed though ActionInfo lists only On")
	}
	if err := d.Reset(context.Background(), system, On); err != nil {
		t.Error(err)
	}
	if len(m.posts) != 1 {
		t.Errorf("posts %+v", m.posts)
	}
}

// TestLive reads a real Redfish service, read-only: DMTF's mockup server, or a
// BMC. It never resets anything.
//
//	WATTPROOF_REDFISH_ENDPOINT=http://127.0.0.1:8000 go test -run TestLive -v ./internal/meter/redfish/
//
// Credentials, if needed, come from WATTPROOF_REDFISH_USER and _PASSWORD.
func TestLive(t *testing.T) {
	ep := os.Getenv("WATTPROOF_REDFISH_ENDPOINT")
	if ep == "" {
		t.Skip("WATTPROOF_REDFISH_ENDPOINT not set")
	}
	d, err := New(context.Background(), Config{
		Endpoint: ep,
		Username: os.Getenv("WATTPROOF_REDFISH_USER"),
		Password: os.Getenv("WATTPROOF_REDFISH_PASSWORD"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metertest.Contract(t, d, 3)
	inv := d.Inventory()
	t.Logf("Redfish %s, vendor %q, firmware %q, systems %q", inv.RedfishVersion, inv.Vendor, inv.Firmware, inv.Systems)
	for _, c := range inv.Chassis {
		t.Logf("chassis %+v", c)
	}
	for _, q := range inv.Quirks {
		t.Logf("quirk: %s", q)
	}
}
