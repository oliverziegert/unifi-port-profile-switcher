## 1. Switcher: ActivePreset function

- [x] 1.1 Add `ActiveResult` struct to `internal/switcher/` with fields `Switch`, `Port`, `ActivePreset` (string, empty when no match), `Profile`, `ProfileID`.
- [x] 1.2 Add `ActivePreset(ctx, cli, presets map[string]config.Preset, switchRef string, port int) (ActiveResult, error)` that calls `Login` → `ListPortProfiles` → `ListDevices`, resolves the device with `unifi.FindDevice`, validates the port with the existing `portExists` helper, reads the port's current `portconf` ID from `port_overrides`, and returns the lexicographically-first preset whose `Switch` resolves to the same device, whose `Port == port`, and whose `Profile` resolves to the current `portconf` ID.
- [x] 1.3 When more than one preset matches the same `(switch, port, profile)`, log at info level with all matching names; still return the lexicographically-first.
- [x] 1.4 When no preset matches, return `ActiveResult{ActivePreset: ""}` with `Profile` and `ProfileID` describing the port's current profile, and no error.

## 2. Switcher: tests

- [x] 2.1 Add `internal/switcher/active_test.go` with a fake `ControllerClient` (reuse pattern from existing `apply_test.go`).
- [x] 2.2 Cover: single-preset match; no match (current profile not in any preset); multiple matching presets returns first alphabetically and logs; switch not found returns `unifi.ErrDeviceNotFound`; port not present returns `switcher.ErrPortOutOfRange`; login failure surfaces `unifi.ErrAuth`.
- [x] 2.3 Assert the function never calls `UpdateDevicePortOverrides` (read-only invariant).

## 3. Server: handler and route

- [x] 3.1 Register `mux.HandleFunc("GET /ports/{switch}/{port}/active", s.auth(s.handleActive))` in `server.New`.
- [x] 3.2 Implement `handleActive`: read `{switch}` via `r.PathValue("switch")`; parse `{port}` with `strconv.Atoi`; return `400 Bad Request` for parse failure or values outside 1–52.
- [x] 3.3 Call `switcher.ActivePreset(r.Context(), cli, s.cfg.Presets, switchRef, port)`; map errors via the existing `writeHTTPError`.
- [x] 3.4 Encode the JSON response: `{"switch", "port", "active_preset" (null when empty), "profile", "profile_id"}`. Use `omitempty` only on `active_preset` if Go's JSON encoder emits it as `null` — otherwise emit it explicitly as `null`.

## 4. Server: tests

- [x] 4.1 Extend `internal/server/server_test.go` with cases for `GET /ports/{switch}/{port}/active`.
- [x] 4.2 Cover: 200 with active preset; 200 with `active_preset: null`; 400 for non-numeric port; 400 for port out of range; 404 for unknown switch; 404 for port not on switch; 401 missing/wrong bearer token; 502 on controller auth failure and APIError; 502 on unreachable controller.

## 5. Docs

- [x] 5.1 Add an "API reference" entry for `GET /ports/{switch}/{port}/active` in `unifi-port-profile-switcher/DOCS.md` (under the existing endpoint documentation, or alongside the Verification section).
- [x] 5.2 Rework the "Highlight the active preset" section in `unifi-port-profile-switcher/DOCS.md` to use **one** `rest` sensor per port (calling the new endpoint, `value_template: {{ value_json.active_preset }}`) instead of one per preset. Update both Option A (`custom:button-card`) and Option B (`tile` + `card_mod`) examples to compare the per-port sensor state against the preset name.
- [x] 5.3 Add the new endpoint to the README endpoint list and the Phase 2 verification block in the root `README.md`.
- [x] 5.4 Add a `CHANGELOG.md` entry under the next unreleased version describing the new endpoint as additive.

## 6. Verification

- [x] 6.1 Run `go test ./...` from the addon directory; all existing and new tests pass.
- [x] 6.2 Run `go vet ./...` and `go build ./...`; no warnings or errors.
- [x] 6.3 Manual smoke test against a live controller: `curl -H "Authorization: Bearer <token>" "http://<ha-ip>:8099/ports/<switch>/<port>/active"` returns the expected JSON for (a) a port currently on a configured preset, (b) a port on a profile not in any preset.
- [x] 6.4 Confirm a second identical call leaves the controller's `port_overrides` unchanged (compare before/after with the existing `/presets/<name>/status` endpoint).
