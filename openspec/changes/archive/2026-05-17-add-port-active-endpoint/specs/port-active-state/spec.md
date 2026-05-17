## ADDED Requirements

### Requirement: Active-preset query endpoint

The add-on HTTP API SHALL expose `GET /ports/{switch}/{port}/active`, which returns the configured preset that is currently active on the given switch and port, together with the port's current port-profile.

The `{switch}` path segment SHALL accept either a UniFi device name (URL-encoded if it contains reserved characters) or a MAC address in colon-separated form, matching the forms accepted by the `presets[].switch` config field.

The `{port}` path segment SHALL be a decimal integer between 1 and 52 inclusive, matching the `presets[].port` config constraint.

The endpoint SHALL require the same bearer-token authentication as `/presets/...` endpoints.

#### Scenario: Returns the matching preset when a configured preset's profile is active on the port

- **WHEN** the controller reports the port's current `portconf` matches the resolved `profile` of a configured preset whose `switch` resolves to the same device and whose `port` equals the requested port
- **THEN** the response status SHALL be `200 OK`
- **AND** the response body SHALL be a JSON object with `active_preset` set to that preset's name, `switch` set to the device's canonical name, `port` set to the requested port, `profile` set to the profile's display name, and `profile_id` set to the profile's `portconf` ID

#### Scenario: Returns null active_preset when no configured preset matches the current profile

- **WHEN** the controller reports the port is on a port-profile that does not correspond to the `profile` of any configured preset for this switch+port
- **THEN** the response status SHALL be `200 OK`
- **AND** the response body's `active_preset` field SHALL be `null`
- **AND** the response body's `profile` and `profile_id` fields SHALL describe the port's actual current profile on the controller

#### Scenario: Returns 400 for an invalid port segment

- **WHEN** the `{port}` path segment is not a decimal integer, or is outside the range 1–52
- **THEN** the response status SHALL be `400 Bad Request`
- **AND** the response body SHALL be `{"error": "<message>"}` describing the parse or range failure

#### Scenario: Returns 404 when the switch is not found

- **WHEN** the `{switch}` segment does not resolve to any device on the configured controller site
- **THEN** the response status SHALL be `404 Not Found`
- **AND** the response body SHALL be `{"error": "<message>"}`

#### Scenario: Returns 404 when the port does not exist on the resolved switch

- **WHEN** the `{switch}` segment resolves to a device whose `port_table` does not contain the requested `{port}`
- **THEN** the response status SHALL be `404 Not Found`
- **AND** the response body SHALL be `{"error": "<message>"}` identifying the switch and port

#### Scenario: Returns 401 when the bearer token is missing or wrong

- **WHEN** the request lacks an `Authorization: Bearer <token>` header, or the token does not match the configured `auth_token`
- **THEN** the response status SHALL be `401 Unauthorized`
- **AND** the response body SHALL be `{"error": "unauthorized"}`

#### Scenario: Returns 502 when the controller cannot be reached or rejects authentication

- **WHEN** the controller login fails, the controller returns a non-2xx response, or the controller is unreachable
- **THEN** the response status SHALL be `502 Bad Gateway`
- **AND** the response body SHALL be `{"error": "<message>"}` describing the upstream failure

### Requirement: Deterministic resolution when multiple presets share switch, port, and profile

When two or more configured presets target the same switch, port, AND port-profile, the endpoint SHALL pick one deterministically rather than returning an arbitrary or non-reproducible answer.

#### Scenario: Two presets with identical switch, port, and profile

- **WHEN** the configuration contains two presets `alpha` and `beta` both targeting the same switch, port, and profile, and that profile is currently active on the port
- **THEN** the response's `active_preset` SHALL be the lexicographically first preset name (`alpha`)
- **AND** the addon SHALL log an info-level message noting that multiple presets matched

### Requirement: Read-only semantics

The endpoint SHALL NOT modify any controller state. It SHALL NOT call `UpdateDevicePortOverrides` or any other write endpoint, regardless of input.

#### Scenario: Repeated calls leave the controller unchanged

- **WHEN** the endpoint is called any number of times for the same switch and port
- **THEN** the controller's `port_overrides` for that device SHALL be byte-identical before and after the calls
