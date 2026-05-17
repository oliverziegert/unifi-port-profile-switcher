package switcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"

	"github.com/oliverziegert/unifi-port-profile-switcher/internal/config"
	"github.com/oliverziegert/unifi-port-profile-switcher/internal/unifi"
)

// ActiveResult is the structured outcome of an ActivePreset call.
//
// ActivePreset is the configured preset name whose profile is currently active
// on the port, or the empty string when no configured preset matches. Profile
// and ProfileID describe the port's current profile on the controller,
// independent of whether any preset matches.
type ActiveResult struct {
	Switch       string
	Port         int
	ActivePreset string
	Profile      string
	ProfileID    string
}

// MarshalJSON emits active_preset as JSON null when no preset matched, rather
// than as an empty string. Dashboards can then distinguish "no match" from a
// preset literally named "".
func (a ActiveResult) MarshalJSON() ([]byte, error) {
	type alias struct {
		Switch       string  `json:"switch"`
		Port         int     `json:"port"`
		ActivePreset *string `json:"active_preset"`
		Profile      string  `json:"profile"`
		ProfileID    string  `json:"profile_id"`
	}
	var active *string
	if a.ActivePreset != "" {
		s := a.ActivePreset
		active = &s
	}
	return json.Marshal(alias{
		Switch:       a.Switch,
		Port:         a.Port,
		ActivePreset: active,
		Profile:      a.Profile,
		ProfileID:    a.ProfileID,
	})
}

// ActivePreset returns the configured preset whose profile is currently active
// on the given switch+port, along with the port's actual current profile.
//
// The function is read-only: it never calls UpdateDevicePortOverrides. When no
// configured preset matches the port's current profile, ActivePreset is the
// empty string and the error is nil.
func ActivePreset(ctx context.Context, cli ControllerClient, presets map[string]config.Preset, switchRef string, port int) (ActiveResult, error) {
	res := ActiveResult{Port: port}

	if err := cli.Login(ctx); err != nil {
		return res, err
	}

	profiles, err := cli.ListPortProfiles(ctx)
	if err != nil {
		return res, err
	}

	device, err := selectDevice(ctx, cli, switchRef)
	if err != nil {
		return res, err
	}
	res.Switch = device.Name

	if !portExists(device, port) {
		return res, fmt.Errorf("%w: switch %q has no port %d", ErrPortOutOfRange, device.Name, port)
	}

	currentID := currentOverrideID(device.PortOverrides, port)
	res.ProfileID = currentID
	res.Profile = unifi.ResolveProfileName(profiles, currentID)

	var matches []string
	for name, p := range presets {
		if p.Port != port {
			continue
		}
		if !unifi.DeviceMatches(*device, p.Switch) {
			continue
		}
		profileID, err := unifi.ResolveProfile(profiles, p.Profile)
		if err != nil {
			continue
		}
		if profileID == currentID && currentID != "" {
			matches = append(matches, name)
		}
	}
	sort.Strings(matches)

	if len(matches) > 1 {
		slog.InfoContext(ctx, "active-preset: multiple presets match",
			"switch", device.Name,
			"port", port,
			"matches", matches,
		)
	}
	if len(matches) > 0 {
		res.ActivePreset = matches[0]
	}
	return res, nil
}
