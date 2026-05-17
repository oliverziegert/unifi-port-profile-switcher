## Context

The add-on exposes an HTTP API consumed by Home Assistant. Today the only way to learn which preset is currently active on a port is to call `GET /presets/<name>/status` and compare `from_profile` to `to_profile`. The existing handler chain — `server.handleStatus` → `switcher.Status` → `unifi.Client.{Login, ListPortProfiles, ListDevices}` — already performs the controller round-trips needed to derive that answer.

Internally, `switcher.Status` resolves the configured preset's `profile` name to its `portconf` ID, fetches the target device, and reads the port's current `portconf` ID from `port_overrides`. The "active" question only needs two facts per request: the port's current `portconf` ID, and the set of configured presets whose `switch` + `port` match. The current per-preset endpoint discards the comparison context that would make the per-port answer cheap.

## Goals / Non-Goals

**Goals:**
- Provide a single endpoint that answers "which configured preset is active on this switch+port?" in one call.
- Reuse the existing controller-client surface (`Login`, `ListPortProfiles`, `ListDevices`) and existing error mapping (`writeHTTPError`) — no new code paths for auth, transport, or HTTP semantics.
- Return a predictable JSON shape that distinguishes "no configured preset matches" from "the port doesn't exist" or "the switch isn't reachable".
- Keep the change additive: existing endpoints, response shapes, and the config schema are untouched.

**Non-Goals:**
- Documenting the entire existing HTTP API as a spec (`/healthz`, `/presets`, `/presets/<name>/status`, `/presets/<name>/apply`). Those are pre-existing surface area without specs today; folding them in inflates scope. They can be backfilled in a follow-up change.
- Caching controller responses across requests. The HA `scan_interval` already bounds call rate.
- Supporting wildcards or "any port on this switch" queries. One switch + one port per request.
- Returning a list of all matching-but-inactive presets in the same response. The dashboard only needs the active one; if other use cases emerge, add a separate endpoint.

## Decisions

### Path shape: `GET /ports/{switch}/{port}/active`

Considered alternatives:
- `GET /ports/{switch}/{port}/state` — "state" implies a richer payload (link, speed, PoE) that this endpoint does not provide. "active" names exactly what the response contains.
- `GET /presets/active?switch=X&port=N` — keeps everything under `/presets`, but the resource being queried is a *port*, not a preset. The active preset is the *answer*, not the subject.
- `GET /switches/{switch}/ports/{port}/active` — more REST-pure but longer. The shorter `/ports/{switch}/{port}/active` matches the project's existing terse style (`/presets/{name}/...`).

Chosen: `/ports/{switch}/{port}/active`. The path's two variables map directly to `Preset.Switch` and `Preset.Port`, the two identifiers used for matching.

### `{switch}` accepts device name or MAC

The existing `Preset.Switch` field already accepts either form, resolved by `unifi.FindDevice`. The new endpoint must accept the same forms to be useful — a user who configured a preset by device name shouldn't have to know its MAC to query state. The path segment is URL-decoded by `http.ServeMux`'s pattern matcher, so names containing spaces ("Office USW-24") work when the client encodes them as `Office%20USW-24`. MAC addresses contain colons, which are reserved in URL paths but unreserved in path segments per RFC 3986 §3.3 and pass through `http.ServeMux` unmodified.

### `{port}` is a decimal integer, validated as 1–52

Matches the add-on's `presets[].port: int(1,52)` schema constraint. The handler parses the segment with `strconv.Atoi` and returns `400 Bad Request` for non-integers or out-of-range values, distinguishing client error from "switch unreachable" (`502`) or "port not present on this hardware" (`404`).

### Response shape

```json
{
  "switch": "Office USW-24",
  "port": 5,
  "active_preset": "work-laptop",
  "profile": "Work VLAN",
  "profile_id": "60a1b2c3d4e5f60718293a4b"
}
```

- `switch` echoes the resolved device name (not the request segment) so the response is self-describing even when the request used a MAC.
- `active_preset` is the name of the configured preset whose `profile` resolves to the port's current `portconf` ID. **`null` when no configured preset matches** — e.g. the port is on a profile not represented in `presets:`. The response is still `200 OK`; "no match" is a normal state for a query endpoint.
- `profile` and `profile_id` describe the port's actual current state on the controller, independent of whether any preset matches. This lets HA template sensors show "current profile" even when `active_preset` is null.

Considered alternative: return `404` when no preset matches. Rejected — that conflates "not found" (the *port* exists, the *switch* exists) with "this query has no matching answer". The dashboard sensor would then need to distinguish "addon down" from "no preset active" at the HTTP-status level, which is fragile.

### New `switcher.ActivePreset` function

Rather than inlining the lookup in the HTTP handler, add `switcher.ActivePreset(ctx, cli, presets, switchRef, port) (ActiveResult, error)` mirroring the shape of `Status` and `Apply`. This keeps the handler thin and gives the same testability properties (the controller client is an interface, fakes already exist in `switcher` tests). The function:

1. Calls `cli.Login`, `cli.ListPortProfiles`, `cli.ListDevices` (same sequence as `Status`).
2. Resolves the device via `unifi.FindDevice` to get the canonical device name and `port_overrides`.
3. Validates the port exists via the existing `portExists` helper.
4. Reads the port's current `portconf` ID from overrides.
5. Filters the caller-supplied preset map to entries where `Preset.Switch` resolves to the same device and `Preset.Port == port`. (Resolution: a preset whose `Switch` is a MAC matches when `device.MAC == preset.Switch`; a preset whose `Switch` is a name matches when `device.Name == preset.Switch`. The same matching rules `unifi.FindDevice` already uses.)
6. Among matching presets, returns the one whose `profile` (resolved via `unifi.ResolveProfile`) equals the port's current `portconf` ID. If none match, returns the result with `ActivePreset == ""` and no error.

### Handler error mapping

Reuse `writeHTTPError` for `unifi.ErrAuth`, `unifi.ErrDeviceNotFound`, `switcher.ErrPortOutOfRange`, and `*unifi.APIError`. The handler adds one new case: `400 Bad Request` when the `{port}` segment fails to parse or is outside 1–52. This is local to the handler, not pushed into `switcher.ActivePreset`, which receives an already-validated `int`.

## Risks / Trade-offs

- **Resolution ambiguity when two presets share switch+port+profile.** Two presets could legally point at the same profile on the same port (e.g. accidental duplication). `ActivePreset` returns the first match in a deterministic order (alphabetical by preset name) and logs at info level when there are multiple matches. → Mitigation: documented; not worth rejecting the config since the controller side is unambiguous and the dashboard answer is still correct (just non-unique on the preset-name axis).
- **Switch name with a `/` in it.** The path pattern `GET /ports/{switch}/{port}/active` would mis-parse if a device name contained `/`. → Mitigation: UniFi device names cannot legally contain `/` in the controller UI; document the expectation and return a clean 404 if the segment doesn't resolve. No code defends against this beyond `unifi.FindDevice`'s existing not-found behaviour.
- **MAC case sensitivity.** UniFi stores MACs lowercased with colon separators. → Mitigation: `unifi.FindDevice` already normalises; no change needed.
- **Cache coherence vs `/presets/<name>/status`.** Both endpoints make their own controller calls; a dashboard mixing both could observe brief inconsistencies. → Mitigation: acceptable; HA sensors poll independently anyway, and the inconsistency window is one controller round-trip.
- **Test coverage for the new function.** `switcher` already has fakes; the new function follows the same shape, so test cost is one new file with a small fake-client setup (a handful of presets and one device with overrides). → Mitigation: budgeted in tasks.

## Migration Plan

Purely additive — no migration. The new endpoint can be released in a normal version bump. `DOCS.md` will be updated in the same change to show the per-port sensor pattern as the recommended approach; the per-preset pattern keeps working for users who haven't updated their `configuration.yaml`.