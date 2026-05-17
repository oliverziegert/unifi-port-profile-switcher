## Why

Dashboards that highlight the currently-active preset must poll `/presets/<name>/status` once per preset. When N presets target the same switch+port, that is N controller round-trips per scan interval to answer a single question: "which preset is active right now?" A per-port endpoint collapses this to one call, regardless of how many presets share the port, and gives Home Assistant a single sensor to bind dashboard styling to.

## What Changes

- Add `GET /ports/{switch}/{port}/active` to the add-on HTTP API. The `{switch}` segment accepts either a device name (URL-encoded) or a MAC address; `{port}` is the 1-based port index.
- The response identifies the active preset by intersecting two facts: the port's current `portconf` on the controller, and the set of configured presets whose `switch` + `port` match the request. The preset whose `profile` resolves to that `portconf` is "active".
- When no configured preset matches the current profile (e.g. the port is on the controller's default profile, or on a profile not represented in `presets:`), the endpoint returns the same shape with `active_preset: null`. This is a normal state, not an error.
- Authentication, error mapping, and logging follow the existing patterns used by `/presets/...` handlers.
- Update `DOCS.md` so the "Highlight the active preset" section uses one `rest` sensor per port instead of one per preset.

## Capabilities

### New Capabilities

- `port-active-state`: query, for a given switch+port, which configured preset (if any) is currently active on the controller.

### Modified Capabilities

<!-- None: existing endpoints are unchanged. -->

## Impact

- Code: `internal/server/server.go` (new route + handler), `internal/switcher/` (new function that returns the active preset name for a switch+port without performing a write), tests alongside both.
- Configuration: no schema changes — existing `presets:` list is the source of truth for which preset names map to which switch+port+profile.
- Docs: `unifi-port-profile-switcher/DOCS.md` "Highlight the active preset" section reworked around the new endpoint. `README.md` endpoint list updated.
- Compatibility: additive only. Existing `/presets/...` endpoints are unchanged. No client breakage.
