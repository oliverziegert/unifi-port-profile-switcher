package switcher_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// fakeClient is the test double for switcher.ControllerClient.
type fakeClient struct {
	profiles    []unifi.PortProfile
	devices     []unifi.Device
	loginErr    error
	listProfErr error
	listDevErr  error
	updateErr   error

	loginCalls    int
	updateCalls   int
	lastDeviceID  string
	lastOverrides []unifi.PortOverride
}

func (f *fakeClient) Login(_ context.Context) error {
	f.loginCalls++
	return f.loginErr
}

func (f *fakeClient) ListPortProfiles(_ context.Context) ([]unifi.PortProfile, error) {
	return f.profiles, f.listProfErr
}

func (f *fakeClient) ListDevices(_ context.Context) ([]unifi.Device, error) {
	return f.devices, f.listDevErr
}

func (f *fakeClient) UpdateDevicePortOverrides(_ context.Context, deviceID string, overrides []unifi.PortOverride) error {
	f.updateCalls++
	f.lastDeviceID = deviceID
	f.lastOverrides = overrides
	return f.updateErr
}

func makeFake() *fakeClient {
	return &fakeClient{
		profiles: []unifi.PortProfile{
			{ID: "work-id", Name: "Work VLAN"},
			{ID: "personal-id", Name: "Personal VLAN"},
		},
		devices: []unifi.Device{
			{
				ID:   "dev-1",
				Name: "Office USW-24",
				MAC:  "aa:bb:cc:dd:ee:ff",
				PortTable: []unifi.Port{
					{PortIDX: 1}, {PortIDX: 2}, {PortIDX: 3}, {PortIDX: 4},
					{PortIDX: 5}, {PortIDX: 6}, {PortIDX: 7}, {PortIDX: 8},
				},
				PortOverrides: []unifi.PortOverride{
					{PortIDX: 3, PortconfID: "guest-id", Rest: map[string]json.RawMessage{"poe_mode": json.RawMessage(`"off"`)}},
					{PortIDX: 5, PortconfID: "personal-id"},
				},
			},
		},
	}
}

var workPreset = config.Preset{Switch: "Office USW-24", Port: 5, Profile: "Work VLAN"}

func TestApply_AlreadyOnTarget_NoOp(t *testing.T) {
	fake := makeFake()
	// Pre-set port 5 to Work VLAN.
	fake.devices[0].PortOverrides[1].PortconfID = "work-id"

	res, err := switcher.Apply(t.Context(), fake, "work-laptop", workPreset, switcher.Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Errorf("Changed = true, want false")
	}
	if res.FromProfile != "Work VLAN" || res.ToProfile != "Work VLAN" {
		t.Errorf("from/to = %q/%q", res.FromProfile, res.ToProfile)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestApply_DifferentProfile_WritesMergedArray(t *testing.T) {
	fake := makeFake()

	res, err := switcher.Apply(t.Context(), fake, "work-laptop", workPreset, switcher.Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Errorf("Changed = false, want true")
	}
	if res.FromProfile != "Personal VLAN" || res.ToProfile != "Work VLAN" {
		t.Errorf("from/to = %q/%q", res.FromProfile, res.ToProfile)
	}
	if fake.updateCalls != 1 {
		t.Fatalf("updateCalls = %d, want 1", fake.updateCalls)
	}
	if fake.lastDeviceID != "dev-1" {
		t.Errorf("deviceID = %q", fake.lastDeviceID)
	}
	if len(fake.lastOverrides) != 2 {
		t.Fatalf("overrides len = %d, want 2", len(fake.lastOverrides))
	}
	// Port 3 must be preserved exactly, including unknown fields.
	port3 := findOverride(fake.lastOverrides, 3)
	if port3 == nil {
		t.Fatalf("port 3 override dropped")
	}
	if port3.PortconfID != "guest-id" {
		t.Errorf("port 3 PortconfID = %q, want guest-id", port3.PortconfID)
	}
	if string(port3.Rest["poe_mode"]) != `"off"` {
		t.Errorf("port 3 lost poe_mode: %v", port3.Rest)
	}
	// Port 5 should be updated.
	port5 := findOverride(fake.lastOverrides, 5)
	if port5 == nil || port5.PortconfID != "work-id" {
		t.Errorf("port 5 = %+v, want PortconfID work-id", port5)
	}
}

func TestApply_PortAbsentFromOverrides_Inserts(t *testing.T) {
	fake := makeFake()
	// Remove the port 5 override.
	fake.devices[0].PortOverrides = fake.devices[0].PortOverrides[:1]

	res, err := switcher.Apply(t.Context(), fake, "work-laptop", workPreset, switcher.Options{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !res.Changed {
		t.Errorf("Changed = false, want true")
	}
	if res.FromProfile != "" {
		t.Errorf("from = %q, want empty (no prior override)", res.FromProfile)
	}
	if fake.updateCalls != 1 {
		t.Fatalf("updateCalls = %d", fake.updateCalls)
	}
	port5 := findOverride(fake.lastOverrides, 5)
	if port5 == nil || port5.PortconfID != "work-id" {
		t.Errorf("port 5 not inserted correctly: %+v", port5)
	}
	if findOverride(fake.lastOverrides, 3) == nil {
		t.Errorf("port 3 override dropped during insert")
	}
}

func TestApply_DryRun_DoesNotWrite(t *testing.T) {
	fake := makeFake()
	res, err := switcher.Apply(t.Context(), fake, "work-laptop", workPreset, switcher.Options{DryRun: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.Changed {
		t.Errorf("Changed = true in dry-run")
	}
	if !res.DryRun {
		t.Errorf("DryRun = false, want true")
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 in dry-run", fake.updateCalls)
	}
}

func TestApply_DeviceNotFound(t *testing.T) {
	fake := makeFake()
	p := workPreset
	p.Switch = "Nope"
	_, err := switcher.Apply(t.Context(), fake, "x", p, switcher.Options{})
	if !errors.Is(err, unifi.ErrDeviceNotFound) {
		t.Fatalf("err = %v, want ErrDeviceNotFound", err)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestApply_ProfileNotFound(t *testing.T) {
	fake := makeFake()
	p := workPreset
	p.Profile = "Guest VLAN"
	_, err := switcher.Apply(t.Context(), fake, "x", p, switcher.Options{})
	if !errors.Is(err, unifi.ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound", err)
	}
}

func TestApply_PortOutOfRange(t *testing.T) {
	fake := makeFake()
	p := workPreset
	p.Port = 99
	_, err := switcher.Apply(t.Context(), fake, "x", p, switcher.Options{})
	if !errors.Is(err, switcher.ErrPortOutOfRange) {
		t.Fatalf("err = %v, want ErrPortOutOfRange", err)
	}
}

func TestStatus_ReturnsCurrentAndTarget(t *testing.T) {
	fake := makeFake()
	res, err := switcher.Status(t.Context(), fake, "work-laptop", workPreset)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if res.FromProfile != "Personal VLAN" || res.ToProfile != "Work VLAN" {
		t.Errorf("from/to = %q/%q", res.FromProfile, res.ToProfile)
	}
	if res.Changed {
		t.Errorf("Changed = true in Status")
	}
	if fake.updateCalls != 0 {
		t.Errorf("Status should not write")
	}
}

func findOverride(overrides []unifi.PortOverride, port int) *unifi.PortOverride {
	for i := range overrides {
		if overrides[i].PortIDX == port {
			return &overrides[i]
		}
	}
	return nil
}
