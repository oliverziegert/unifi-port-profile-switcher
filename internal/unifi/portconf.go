package unifi

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// PortProfile is the subset of a port profile we care about.
type PortProfile struct {
	ID   string `json:"_id"`
	Name string `json:"name"`
}

// ErrProfileNotFound is returned when ResolveProfile finds no matching profile.
var ErrProfileNotFound = errors.New("unifi: port profile not found")

// ListPortProfiles returns the port profiles configured on the controller's site.
func (c *Client) ListPortProfiles(ctx context.Context) ([]PortProfile, error) {
	var out []PortProfile
	if err := c.Do(ctx, "GET", c.sitePath("/rest/portconf"), nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveProfile returns the ID of the profile whose name matches exactly.
// When no match is found it returns ErrProfileNotFound and includes the
// available profile names in the error message.
func ResolveProfile(profiles []PortProfile, name string) (string, error) {
	for _, p := range profiles {
		if p.Name == name {
			return p.ID, nil
		}
	}
	available := make([]string, 0, len(profiles))
	for _, p := range profiles {
		available = append(available, p.Name)
	}
	sort.Strings(available)
	return "", fmt.Errorf("%w: %q (available: %s)", ErrProfileNotFound, name, strings.Join(available, ", "))
}

// ResolveProfileName is the inverse: from an ID, find a human-readable name.
// Returns the empty string when no match is found.
func ResolveProfileName(profiles []PortProfile, id string) string {
	for _, p := range profiles {
		if p.ID == id {
			return p.Name
		}
	}
	return ""
}
