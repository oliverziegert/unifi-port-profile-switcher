package unifi_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

func TestPortOverride_RoundTripPreservesUnknownFields(t *testing.T) {
	input := `{
		"port_idx": 5,
		"portconf_id": "profile-work",
		"poe_mode": "auto",
		"name": "Dock",
		"op_mode": "switch"
	}`
	var o unifi.PortOverride
	if err := json.Unmarshal([]byte(input), &o); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if o.PortIDX != 5 || o.PortconfID != "profile-work" {
		t.Errorf("typed fields = %+v", o)
	}
	if string(o.Rest["poe_mode"]) != `"auto"` ||
		string(o.Rest["name"]) != `"Dock"` ||
		string(o.Rest["op_mode"]) != `"switch"` {
		t.Errorf("Rest = %v", o.Rest)
	}

	out, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	for _, want := range []string{`"port_idx":5`, `"portconf_id":"profile-work"`, `"poe_mode":"auto"`, `"name":"Dock"`, `"op_mode":"switch"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("output missing %q in %s", want, out)
		}
	}
}

func TestPortOverride_TypedFieldsBeatRest(t *testing.T) {
	o := unifi.PortOverride{
		PortIDX:    7,
		PortconfID: "real",
		Rest:       map[string]json.RawMessage{"port_idx": json.RawMessage("99"), "portconf_id": json.RawMessage(`"fake"`)},
	}
	out, err := json.Marshal(o)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), `"port_idx":7`) || !strings.Contains(string(out), `"portconf_id":"real"`) {
		t.Errorf("typed fields not authoritative: %s", out)
	}
}

func TestFindDevice_ByName(t *testing.T) {
	devices := []unifi.Device{
		{Name: "Office USW-24", MAC: "aa:bb:cc:dd:ee:ff", ID: "1"},
		{Name: "Living USW-Flex", MAC: "11:22:33:44:55:66", ID: "2"},
	}
	d, err := unifi.FindDevice(devices, "office usw-24")
	if err != nil {
		t.Fatalf("FindDevice: %v", err)
	}
	if d.ID != "1" {
		t.Errorf("ID = %q, want 1", d.ID)
	}
}

func TestFindDevice_ByMAC(t *testing.T) {
	devices := []unifi.Device{
		{Name: "x", MAC: "aa:bb:cc:dd:ee:ff", ID: "1"},
	}
	d, err := unifi.FindDevice(devices, "AA-BB-CC-DD-EE-FF")
	if err != nil {
		t.Fatalf("FindDevice MAC: %v", err)
	}
	if d.ID != "1" {
		t.Errorf("ID = %q", d.ID)
	}
}

func TestFindDevice_NotFound(t *testing.T) {
	devices := []unifi.Device{
		{Name: "Office USW-24", MAC: "aa:bb:cc:dd:ee:ff", ID: "1"},
	}
	_, err := unifi.FindDevice(devices, "Unknown")
	if !errors.Is(err, unifi.ErrDeviceNotFound) {
		t.Fatalf("err = %v, want ErrDeviceNotFound", err)
	}
	if !strings.Contains(err.Error(), "Office USW-24") {
		t.Errorf("error should list available devices, got %v", err)
	}
}

func TestResolveProfile(t *testing.T) {
	profiles := []unifi.PortProfile{
		{ID: "a", Name: "Work VLAN"},
		{ID: "b", Name: "Personal VLAN"},
	}
	id, err := unifi.ResolveProfile(profiles, "Work VLAN")
	if err != nil {
		t.Fatalf("ResolveProfile: %v", err)
	}
	if id != "a" {
		t.Errorf("id = %q, want a", id)
	}

	_, err = unifi.ResolveProfile(profiles, "Guest VLAN")
	if !errors.Is(err, unifi.ErrProfileNotFound) {
		t.Fatalf("err = %v, want ErrProfileNotFound", err)
	}
	if !strings.Contains(err.Error(), "Personal VLAN") || !strings.Contains(err.Error(), "Work VLAN") {
		t.Errorf("error should list available profiles, got %v", err)
	}
}

func TestResolveProfileName(t *testing.T) {
	profiles := []unifi.PortProfile{{ID: "a", Name: "Work VLAN"}}
	if got := unifi.ResolveProfileName(profiles, "a"); got != "Work VLAN" {
		t.Errorf("name = %q", got)
	}
	if got := unifi.ResolveProfileName(profiles, "missing"); got != "" {
		t.Errorf("name for missing id = %q, want empty", got)
	}
}
