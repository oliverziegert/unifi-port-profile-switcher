package unifi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ErrDeviceNotFound is returned by FindDevice when no device matches.
var ErrDeviceNotFound = errors.New("unifi: device not found")

// Device is the subset of a UniFi device we need for port-profile mutation.
type Device struct {
	ID            string         `json:"_id"`
	Name          string         `json:"name"`
	MAC           string         `json:"mac"`
	PortTable     []Port         `json:"port_table"`
	PortOverrides []PortOverride `json:"port_overrides"`
}

// Port describes one physical port from the device's port_table.
type Port struct {
	PortIDX int    `json:"port_idx"`
	Name    string `json:"name"`
}

// PortOverride is one entry in a device's port_overrides array.
// PortIDX and PortconfID are the only fields we manage; every other field is
// captured in Rest and round-tripped verbatim during PUT so we never lose
// settings (PoE mode, port name, operation mode, ...) configured elsewhere.
type PortOverride struct {
	PortIDX    int
	PortconfID string
	Rest       map[string]json.RawMessage
}

// MarshalJSON serializes the override as a flat object: PortIDX and PortconfID
// are emitted first, then any keys in Rest. Keys present in both lose to the
// typed fields.
func (p PortOverride) MarshalJSON() ([]byte, error) {
	out := make(map[string]json.RawMessage, len(p.Rest)+2)
	for k, v := range p.Rest {
		if k == "port_idx" || k == "portconf_id" {
			continue
		}
		out[k] = v
	}
	idx, err := json.Marshal(p.PortIDX)
	if err != nil {
		return nil, err
	}
	out["port_idx"] = idx
	if p.PortconfID != "" {
		id, err := json.Marshal(p.PortconfID)
		if err != nil {
			return nil, err
		}
		out["portconf_id"] = id
	}
	return json.Marshal(out)
}

// UnmarshalJSON splits the flat object into the typed fields and the Rest map.
func (p *PortOverride) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := raw["port_idx"]; ok {
		if err := json.Unmarshal(v, &p.PortIDX); err != nil {
			return fmt.Errorf("unifi: decode port_idx: %w", err)
		}
		delete(raw, "port_idx")
	}
	if v, ok := raw["portconf_id"]; ok {
		if err := json.Unmarshal(v, &p.PortconfID); err != nil {
			return fmt.Errorf("unifi: decode portconf_id: %w", err)
		}
		delete(raw, "portconf_id")
	}
	if len(raw) > 0 {
		p.Rest = raw
	} else {
		p.Rest = nil
	}
	return nil
}

// ListDevices returns all devices on the configured site.
func (c *Client) ListDevices(ctx context.Context) ([]Device, error) {
	var out []Device
	if err := c.Do(ctx, "GET", c.sitePath("/stat/device"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

var macRE = regexp.MustCompile(`^([0-9a-fA-F]{2}[:.-]?){5}[0-9a-fA-F]{2}$`)

// FindDevice picks the device matching ref. A MAC-like ref is matched against
// the device's MAC (case- and separator-insensitive); otherwise ref is matched
// against the device name, case-insensitively.
func FindDevice(devices []Device, ref string) (*Device, error) {
	if macRE.MatchString(ref) {
		want := normalizeMAC(ref)
		for i := range devices {
			if normalizeMAC(devices[i].MAC) == want {
				return &devices[i], nil
			}
		}
	} else {
		want := strings.ToLower(ref)
		for i := range devices {
			if strings.ToLower(devices[i].Name) == want {
				return &devices[i], nil
			}
		}
	}
	names := make([]string, 0, len(devices))
	for _, d := range devices {
		if d.Name != "" {
			names = append(names, d.Name)
		}
	}
	sort.Strings(names)
	return nil, fmt.Errorf("%w: %q (available: %s)", ErrDeviceNotFound, ref, strings.Join(names, ", "))
}

func normalizeMAC(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
}

// UpdateDevicePortOverrides PUTs a new port_overrides array for the given device.
func (c *Client) UpdateDevicePortOverrides(ctx context.Context, deviceID string, overrides []PortOverride) error {
	body := map[string]any{"port_overrides": overrides}
	return c.Do(ctx, "PUT", c.sitePath("/rest/device/"+deviceID), body, nil)
}
