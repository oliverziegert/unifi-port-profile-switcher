# UniFi Port Profile Switcher — Add-on Documentation

## Installation

1. In Home Assistant, go to **Settings → Add-ons → Add-on Store**.
2. Click the three-dot menu (top right) and choose **Repositories**.
3. Add the repository URL: `https://github.com/oliverziegert/unifi-port-profile-switcher`
4. Find **UniFi Port Profile Switcher** in the store and click **Install**.
5. Open the **Configuration** tab, fill in all options (see below), and click **Save**.
6. Start the add-on. Check the **Log** tab to confirm it printed `listening on ...`.

## Options Reference

| Option                    | Type     | Description                                                       |
|---------------------------|----------|-------------------------------------------------------------------|
| `controller_url`          | string   | Full URL of your UniFi OS controller, e.g. `https://192.168.1.1`  |
| `controller_site`         | string   | UniFi site name (default: `default`)                              |
| `controller_username`     | string   | Local admin user created for this tool                            |
| `controller_password`     | password | Password for the controller user                                  |
| `controller_insecure_tls` | bool     | Skip TLS verification (needed for self-signed certs)              |
| `auth_token`              | password | Bearer token for API authentication — store in `secrets.yaml`     |
| `presets`                 | list     | Named presets; each entry has `name`, `switch`, `port`, `profile` |

**Example preset list:**

```yaml
presets:
  - name: work-laptop
    switch: "Office USW-24"
    port: 5
    profile: "Work VLAN"
  - name: personal-laptop
    switch: "Office USW-24"
    port: 5
    profile: "Personal VLAN"
```

## Home Assistant Wiring

### 1. Store the bearer token as a secret

In `/config/secrets.yaml`:

```yaml
unifi_switcher_token: "Bearer your-long-random-bearer-token-here"
```

### 2. Add `rest_command` entries

In `configuration.yaml`:

```yaml
rest_command:
  unifi_presets_list:
    url: "http://cc48f239-unifi-port-profile-switcher:8099/presets"
    method: GET
    headers:
      Authorization: !secret unifi_switcher_token

  unifi_preset_status:
    url: "http://cc48f239-unifi-port-profile-switcher:8099/presets/{{ preset }}/status"
    method: GET
    headers:
      Authorization: !secret unifi_switcher_token

  unifi_preset_apply:
    url: "http://cc48f239-unifi-port-profile-switcher:8099/presets/{{ preset }}/apply"
    method: POST
    headers:
      Authorization: !secret unifi_switcher_token
```

> **Note:** The internal hostname `cc48f239-unifi-port-profile-switcher` is derived from the add-on slug. HA Supervisor
> creates a DNS entry for each running add-on.

### 3. Add `script` definitions

```yaml
script:
  switch_to_work_laptop:
    alias: "Switch to Work Laptop"
    sequence:
      - service: rest_command.unifi_preset_apply
        data:
          preset: work-laptop

  switch_to_personal_laptop:
    alias: "Switch to Personal Laptop"
    sequence:
      - service: rest_command.unifi_preset_apply
        data:
          preset: personal-laptop
```

### 4. Add a Lovelace button card

In your dashboard YAML:

```yaml
type: button
name: Work Laptop
icon: mdi:laptop
tap_action:
  action: call-service
  service: script.switch_to_work_laptop
```

Or with a second button:

```yaml
type: horizontal-stack
cards:
  - type: button
    name: Work
    icon: mdi:briefcase
    tap_action:
      action: call-service
      service: script.switch_to_work_laptop
  - type: button
    name: Personal
    icon: mdi:home
    tap_action:
      action: call-service
      service: script.switch_to_personal_laptop
```

### 5. Highlight the active preset (optional)

To make the dashboard reflect which preset is currently active, add **one** `rest` sensor per **port** (not per preset) that polls `GET /ports/<switch>/<port>/active`. The sensor's state is the name of the active preset, or the string `None` when no configured preset matches the port's current profile. Compare that state against each preset name in your card configs. Choose **one** of the two dashboard styles below.

#### Sensors

In `configuration.yaml`:

```yaml
sensor:
  - platform: rest
    name: unifi_port_office_usw24_5
    resource: http://cc48f239-unifi-port-profile-switcher:8099/ports/Office%20USW-24/5/active
    headers:
      Authorization: !secret unifi_switcher_token
    scan_interval: 30
    value_template: "{{ value_json.active_preset }}"
```

One sensor per port covers every preset that targets that port. If the dock is shared across two switches, add one sensor per switch+port pair; if multiple ports use the same set of presets, add one sensor per port.

#### Option A — `custom:button-card` (HACS)

Replace the buttons from step 4 with:

```yaml
type: horizontal-stack
cards:
  - type: custom:button-card
    name: Work
    icon: mdi:briefcase
    entity: sensor.unifi_port_office_usw24_5
    show_state: false
    tap_action:
      action: call-service
      service: script.switch_to_work_laptop
    state:
      - value: "work-laptop"
        styles:
          card:
            - border: 2px solid var(--primary-color)
            - background-color: rgba(3, 169, 244, 0.15)
          icon:
            - color: var(--primary-color)
          name:
            - font-weight: bold
      - operator: default
        styles:
          icon:
            - color: var(--secondary-text-color)

  - type: custom:button-card
    name: Personal
    icon: mdi:home
    entity: sensor.unifi_port_office_usw24_5
    show_state: false
    tap_action:
      action: call-service
      service: script.switch_to_personal_laptop
    state:
      - value: "personal-laptop"
        styles:
          card:
            - border: 2px solid var(--primary-color)
            - background-color: rgba(3, 169, 244, 0.15)
          icon:
            - color: var(--primary-color)
          name:
            - font-weight: bold
      - operator: default
        styles:
          icon:
            - color: var(--secondary-text-color)
```

Both buttons read from the **same** sensor and compare its state to their own preset name. For many presets, lift the repeated `state:` block into a `button_card_templates:` entry at the dashboard root and reference it with `template: preset_button` to avoid duplication.

#### Option B — Built-in `tile` card with `card_mod` (HACS for `card_mod` only)

```yaml
type: horizontal-stack
cards:
  - type: tile
    entity: sensor.unifi_port_office_usw24_5
    name: Work
    icon: mdi:briefcase
    tap_action:
      action: call-service
      service: script.switch_to_work_laptop
    card_mod:
      style: |
        ha-card {
          {% if is_state(config.entity, 'work-laptop') %}
          border: 2px solid var(--primary-color);
          background: rgba(3, 169, 244, 0.15);
          {% else %}
          border: 2px solid transparent;
          {% endif %}
        }
        ha-tile-icon {
          {% if is_state(config.entity, 'work-laptop') %}
          --tile-color: var(--primary-color);
          {% endif %}
        }

  - type: tile
    entity: sensor.unifi_port_office_usw24_5
    name: Personal
    icon: mdi:home
    tap_action:
      action: call-service
      service: script.switch_to_personal_laptop
    card_mod:
      style: |
        ha-card {
          {% if is_state(config.entity, 'personal-laptop') %}
          border: 2px solid var(--primary-color);
          background: rgba(3, 169, 244, 0.15);
          {% else %}
          border: 2px solid transparent;
          {% endif %}
        }
        ha-tile-icon {
          {% if is_state(config.entity, 'personal-laptop') %}
          --tile-color: var(--primary-color);
          {% endif %}
        }
```

#### Snap the highlight after pressing a button

The default `scan_interval` of 30 s means the highlight only updates on the next poll. To refresh it immediately after a switch, extend the scripts from step 3 to force-update the per-port sensor:

```yaml
script:
  switch_to_work_laptop:
    alias: "Switch to Work Laptop"
    sequence:
      - service: rest_command.unifi_preset_apply
        data:
          preset: work-laptop
      - service: homeassistant.update_entity
        target:
          entity_id: sensor.unifi_port_office_usw24_5
```

Because every preset on that port now reads from the same sensor, you only need to refresh one entity regardless of how many presets the port hosts.

## API reference

| Method | Path                                | Auth   | Purpose                                                                |
|--------|-------------------------------------|--------|------------------------------------------------------------------------|
| GET    | `/healthz`                          | none   | Liveness probe — returns `{"ok": true}`.                               |
| GET    | `/presets`                          | bearer | List all configured presets.                                           |
| GET    | `/presets/{name}/status`            | bearer | Show the named preset's target profile and the port's current profile. |
| POST   | `/presets/{name}/apply`             | bearer | Apply the named preset (idempotent). `?dry_run=1` to preview only.     |
| GET    | `/ports/{switch}/{port}/active`     | bearer | Read-only: which configured preset is currently active on this port.   |

### `GET /ports/{switch}/{port}/active`

Returns the configured preset whose profile is currently active on the given switch and port, together with the port's current profile.

- `{switch}` accepts either a UniFi device name (URL-encode reserved characters) or a MAC address in colon-separated form. Same forms as `presets[].switch`.
- `{port}` is a decimal integer between 1 and 52 inclusive.
- The endpoint never modifies controller state. It is safe to poll.

Response (`200 OK`):

```json
{
  "switch": "Office USW-24",
  "port": 5,
  "active_preset": "work-laptop",
  "profile": "Work VLAN",
  "profile_id": "60a1b2c3d4e5f60718293a4b"
}
```

When no configured preset matches the port's current profile, the response is still `200 OK` with `"active_preset": null`. `profile` and `profile_id` still describe the port's actual current state on the controller.

Error responses (all return `{"error": "<message>"}`):

| Status | When                                                                |
|--------|---------------------------------------------------------------------|
| 400    | `{port}` is not an integer, or is outside 1–52.                     |
| 401    | Missing or wrong bearer token.                                      |
| 404    | `{switch}` does not resolve, or the port is not on that switch.     |
| 502    | Controller login failed, returned non-2xx, or was unreachable.      |

When two or more presets share the same switch, port, and profile, the endpoint returns the lexicographically-first preset name and logs an info-level message naming all matches.

## Verification

After installation and configuration, verify end-to-end connectivity:

1. **Healthcheck** (no auth required):
   ```sh
   curl http://<ha-ip>:8099/healthz
   # → {"ok":true}
   ```

2. **List presets**:
   ```sh
   curl -H "Authorization: Bearer <token>" http://<ha-ip>:8099/presets
   ```

3. **Dry-run apply** (reads current state, no write):
   ```sh
   curl -X POST -H "Authorization: Bearer <token>" \
     "http://<ha-ip>:8099/presets/work-laptop/apply?dry_run=1"
   ```

4. **Real apply**:
   ```sh
   curl -X POST -H "Authorization: Bearer <token>" \
     http://<ha-ip>:8099/presets/work-laptop/apply
   # → {"changed":true,...}
   ```

5. **Idempotent re-run** — run the same apply again; `changed` should be `false`.

6. **Active-preset lookup** — confirm the dashboard sensor's view of which preset is active:
   ```sh
   curl -H "Authorization: Bearer <token>" \
     "http://<ha-ip>:8099/ports/Office%20USW-24/5/active"
   # → {"switch":"Office USW-24","port":5,"active_preset":"work-laptop",...}
   ```

## Upgrading from 2.x

Version 3.0.0 switches the container base image from Debian to Alpine (via the official HA base image) and tightens the
options schema. Supervisor will prompt you to **reconfigure** after installing the update.

**What you need to do:**

1. Open the **Configuration** tab after the update installs.
2. Review your settings — Supervisor will highlight any values that no longer match the stricter schema.
3. Click **Save** and start the add-on.

**Schema changes that may require attention:**

| Option            | Old   | New                        | Notes                                   |
|-------------------|-------|----------------------------|-----------------------------------------|
| `controller_url`  | `str` | `url(http,https)`          | Must start with `http://` or `https://` |
| `controller_site` | `str` | `match(^[A-Za-z0-9_-]+$)?` | Alnum, dash, underscore only            |
| `presets[].name`  | `str` | `match(^[a-z0-9-]+$)`      | Lowercase alnum and dash only           |
| `presets[].port`  | `int` | `int(1,52)`                | Must be between 1 and 52                |

**What stays the same:**

- Internal hostname: `cc48f239-unifi-port-profile-switcher`
- Exposed port: `8099/tcp`
- Bearer-token authentication model
- All HTTP endpoints (`/healthz`, `/presets`, `/presets/<name>/apply`, `/presets/<name>/status`)
- Existing `rest_command` and Lovelace wiring requires no changes

## Troubleshooting

- **Add-on not appearing in store**: Make sure the repository URL was added exactly as shown. Refresh the store.
- **`listening on ...` not in logs**: Check the **Configuration** tab — `auth_token` must not be empty.
- **401 from HA automation**: Confirm the bearer token in `secrets.yaml` matches the `auth_token` option exactly.
- **502 from apply**: The add-on cannot reach the UniFi controller. Check `controller_url` and credentials; if using a
  self-signed cert, enable `controller_insecure_tls`.
