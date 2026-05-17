package switcher_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/switcher"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

func makeActivePresets() map[string]config.Preset {
	return map[string]config.Preset{
		"work-laptop":     {Switch: "Office USW-24", Port: 5, Profile: "Work VLAN"},
		"personal-laptop": {Switch: "Office USW-24", Port: 5, Profile: "Personal VLAN"},
		"guest-port":      {Switch: "Office USW-24", Port: 3, Profile: "Personal VLAN"},
	}
}

func TestActivePreset_SingleMatch(t *testing.T) {
	fake := makeFake()
	// Port 5 override defaults to personal-id in makeFake.
	res, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", 5)
	if err != nil {
		t.Fatalf("ActivePreset: %v", err)
	}
	if res.ActivePreset != "personal-laptop" {
		t.Errorf("ActivePreset = %q, want personal-laptop", res.ActivePreset)
	}
	if res.Switch != "Office USW-24" {
		t.Errorf("Switch = %q", res.Switch)
	}
	if res.Port != 5 {
		t.Errorf("Port = %d, want 5", res.Port)
	}
	if res.Profile != "Personal VLAN" || res.ProfileID != "personal-id" {
		t.Errorf("Profile/ID = %q/%q", res.Profile, res.ProfileID)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0 (read-only)", fake.updateCalls)
	}
}

func TestActivePreset_MatchByMAC(t *testing.T) {
	fake := makeFake()
	res, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "aa:bb:cc:dd:ee:ff", 5)
	if err != nil {
		t.Fatalf("ActivePreset: %v", err)
	}
	if res.ActivePreset != "personal-laptop" {
		t.Errorf("ActivePreset = %q, want personal-laptop", res.ActivePreset)
	}
	// Switch in result is the canonical device name, not the MAC the caller used.
	if res.Switch != "Office USW-24" {
		t.Errorf("Switch = %q, want Office USW-24", res.Switch)
	}
}

func TestActivePreset_NoMatch_CurrentProfileNotInPresets(t *testing.T) {
	fake := makeFake()
	// Pre-set port 5 to a profile that is not the target of any preset.
	fake.devices[0].PortOverrides[1].PortconfID = "guest-id"
	// Add the unrelated profile so it resolves to a name.
	fake.profiles = append(fake.profiles, unifi.PortProfile{ID: "guest-id", Name: "Guest VLAN"})

	res, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", 5)
	if err != nil {
		t.Fatalf("ActivePreset: %v", err)
	}
	if res.ActivePreset != "" {
		t.Errorf("ActivePreset = %q, want empty", res.ActivePreset)
	}
	if res.Profile != "Guest VLAN" || res.ProfileID != "guest-id" {
		t.Errorf("Profile/ID = %q/%q, want Guest VLAN/guest-id", res.Profile, res.ProfileID)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestActivePreset_NoOverride_NoMatch(t *testing.T) {
	fake := makeFake()
	// Drop the port 5 override entirely — port is on the controller default.
	fake.devices[0].PortOverrides = fake.devices[0].PortOverrides[:1]

	res, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", 5)
	if err != nil {
		t.Fatalf("ActivePreset: %v", err)
	}
	if res.ActivePreset != "" {
		t.Errorf("ActivePreset = %q, want empty when port has no override", res.ActivePreset)
	}
	if res.ProfileID != "" {
		t.Errorf("ProfileID = %q, want empty when port has no override", res.ProfileID)
	}
}

func TestActivePreset_MultipleMatches_ReturnsLexicographicallyFirst(t *testing.T) {
	fake := makeFake()
	// Make port 5 sit on Work VLAN, and add a second preset also pointing at
	// (Office USW-24, 5, Work VLAN). The lexicographically-first match wins.
	fake.devices[0].PortOverrides[1].PortconfID = "work-id"

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	presets := makeActivePresets()
	presets["alpha"] = config.Preset{Switch: "Office USW-24", Port: 5, Profile: "Work VLAN"}
	presets["work-laptop"] = config.Preset{Switch: "Office USW-24", Port: 5, Profile: "Work VLAN"}

	res, err := switcher.ActivePreset(t.Context(), fake, presets, "Office USW-24", 5)
	if err != nil {
		t.Fatalf("ActivePreset: %v", err)
	}
	if res.ActivePreset != "alpha" {
		t.Errorf("ActivePreset = %q, want alpha (lexicographically first)", res.ActivePreset)
	}

	log := buf.String()
	if !strings.Contains(log, "multiple presets match") {
		t.Errorf("expected info log about multiple matches, got: %s", log)
	}
	if !strings.Contains(log, "alpha") || !strings.Contains(log, "work-laptop") {
		t.Errorf("log should mention all matching names, got: %s", log)
	}
}

func TestActivePreset_DeviceNotFound(t *testing.T) {
	fake := makeFake()
	_, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Nope", 5)
	if !errors.Is(err, unifi.ErrDeviceNotFound) {
		t.Fatalf("err = %v, want ErrDeviceNotFound", err)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestActivePreset_PortNotPresent(t *testing.T) {
	fake := makeFake()
	_, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", 24)
	if !errors.Is(err, switcher.ErrPortOutOfRange) {
		t.Fatalf("err = %v, want ErrPortOutOfRange", err)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestActivePreset_LoginFailure(t *testing.T) {
	fake := makeFake()
	fake.loginErr = unifi.ErrAuth
	_, err := switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", 5)
	if !errors.Is(err, unifi.ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
	if fake.updateCalls != 0 {
		t.Errorf("updateCalls = %d, want 0", fake.updateCalls)
	}
}

func TestActivePreset_NeverWrites(t *testing.T) {
	fake := makeFake()
	// Run several scenarios in sequence; updateCalls must remain 0 throughout.
	for _, p := range []int{1, 5, 8} {
		_, _ = switcher.ActivePreset(t.Context(), fake, makeActivePresets(), "Office USW-24", p)
	}
	if fake.updateCalls != 0 {
		t.Errorf("read-only invariant violated: updateCalls = %d", fake.updateCalls)
	}
}

func TestActiveResult_JSON_NullWhenNoMatch(t *testing.T) {
	r := switcher.ActiveResult{
		Switch:    "Office USW-24",
		Port:      5,
		Profile:   "Guest VLAN",
		ProfileID: "guest-id",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"active_preset":null`) {
		t.Errorf("active_preset not emitted as null: %s", b)
	}
}

func TestActiveResult_JSON_StringWhenMatch(t *testing.T) {
	r := switcher.ActiveResult{
		Switch:       "Office USW-24",
		Port:         5,
		ActivePreset: "work-laptop",
		Profile:      "Work VLAN",
		ProfileID:    "work-id",
	}
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"active_preset":"work-laptop"`) {
		t.Errorf("active_preset wrong: %s", b)
	}
}

