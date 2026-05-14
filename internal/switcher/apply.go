// Package switcher applies a named preset to a UniFi switch port.
//
// Apply is idempotent: it fetches the current port_overrides array, mutates
// only the entry for the preset's port (inserting one if absent), and PUTs
// the merged array back. When the port is already on the target profile it
// returns without contacting the controller's write endpoint.
package switcher

import (
	"context"
	"errors"
	"fmt"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// ControllerClient is the surface of unifi.Client used here. Tests inject a fake.
type ControllerClient interface {
	Login(ctx context.Context) error
	ListPortProfiles(ctx context.Context) ([]unifi.PortProfile, error)
	ListDevices(ctx context.Context) ([]unifi.Device, error)
	UpdateDevicePortOverrides(ctx context.Context, deviceID string, overrides []unifi.PortOverride) error
}

// Options tweaks Apply's behavior.
type Options struct {
	DryRun bool
}

// Result is the structured outcome of an Apply or Status call. Field names are
// chosen to match the JSON keys defined in the spec for --json output.
type Result struct {
	Preset      string `json:"preset"`
	Switch      string `json:"switch"`
	Port        int    `json:"port"`
	FromProfile string `json:"from_profile"`
	ToProfile   string `json:"to_profile"`
	Changed     bool   `json:"changed"`
	DryRun      bool   `json:"dry_run"`
}

// ErrPortOutOfRange is returned when the preset targets a port_idx beyond
// what the switch reports in port_table.
var ErrPortOutOfRange = errors.New("switcher: port out of range for switch")

// Apply ensures the preset's target port profile is active on the configured port.
func Apply(ctx context.Context, cli ControllerClient, name string, preset config.Preset, opts Options) (Result, error) {
	res := Result{
		Preset:    name,
		Switch:    preset.Switch,
		Port:      preset.Port,
		ToProfile: preset.Profile,
		DryRun:    opts.DryRun,
	}

	if err := cli.Login(ctx); err != nil {
		return res, err
	}

	profiles, err := cli.ListPortProfiles(ctx)
	if err != nil {
		return res, err
	}
	targetID, err := unifi.ResolveProfile(profiles, preset.Profile)
	if err != nil {
		return res, err
	}

	device, err := selectDevice(ctx, cli, preset.Switch)
	if err != nil {
		return res, err
	}
	if !portExists(device, preset.Port) {
		return res, fmt.Errorf("%w: switch %q has no port %d", ErrPortOutOfRange, device.Name, preset.Port)
	}

	currentID := currentOverrideID(device.PortOverrides, preset.Port)
	res.FromProfile = unifi.ResolveProfileName(profiles, currentID)

	if currentID == targetID {
		// Already on the target profile - no write.
		return res, nil
	}

	if opts.DryRun {
		res.Changed = false
		return res, nil
	}

	overrides := mergeOverride(device.PortOverrides, preset.Port, targetID)
	if err := cli.UpdateDevicePortOverrides(ctx, device.ID, overrides); err != nil {
		return res, err
	}
	res.Changed = true
	return res, nil
}

// Status returns the current and target profiles for the preset without writing.
func Status(ctx context.Context, cli ControllerClient, name string, preset config.Preset) (Result, error) {
	res := Result{
		Preset:    name,
		Switch:    preset.Switch,
		Port:      preset.Port,
		ToProfile: preset.Profile,
	}

	if err := cli.Login(ctx); err != nil {
		return res, err
	}

	profiles, err := cli.ListPortProfiles(ctx)
	if err != nil {
		return res, err
	}
	if _, err := unifi.ResolveProfile(profiles, preset.Profile); err != nil {
		return res, err
	}

	device, err := selectDevice(ctx, cli, preset.Switch)
	if err != nil {
		return res, err
	}
	if !portExists(device, preset.Port) {
		return res, fmt.Errorf("%w: switch %q has no port %d", ErrPortOutOfRange, device.Name, preset.Port)
	}

	currentID := currentOverrideID(device.PortOverrides, preset.Port)
	res.FromProfile = unifi.ResolveProfileName(profiles, currentID)
	return res, nil
}

func selectDevice(ctx context.Context, cli ControllerClient, ref string) (*unifi.Device, error) {
	devices, err := cli.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	return unifi.FindDevice(devices, ref)
}

func portExists(d *unifi.Device, port int) bool {
	if len(d.PortTable) == 0 {
		// Some firmware omits port_table; trust the user.
		return true
	}
	for _, p := range d.PortTable {
		if p.PortIDX == port {
			return true
		}
	}
	return false
}

func currentOverrideID(overrides []unifi.PortOverride, port int) string {
	for _, o := range overrides {
		if o.PortIDX == port {
			return o.PortconfID
		}
	}
	return ""
}

// mergeOverride returns a new slice in which the entry for port carries
// portconfID, inserting a new entry when none exists. Other entries are
// preserved byte-identically.
func mergeOverride(overrides []unifi.PortOverride, port int, portconfID string) []unifi.PortOverride {
	out := make([]unifi.PortOverride, 0, len(overrides)+1)
	found := false
	for _, o := range overrides {
		if o.PortIDX == port {
			o.PortconfID = portconfID
			found = true
		}
		out = append(out, o)
	}
	if !found {
		out = append(out, unifi.PortOverride{PortIDX: port, PortconfID: portconfID})
	}
	return out
}
