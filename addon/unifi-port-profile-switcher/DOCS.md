# UniFi Port Profile Switcher — Add-on Documentation

## Installation

1. In Home Assistant, go to **Settings → Add-ons → Add-on Store**.
2. Click the three-dot menu (top right) and choose **Repositories**.
3. Add the repository URL: `https://github.com/oliverziegert/unifi-port-profile-switcher`
4. Find **UniFi Port Profile Switcher** in the store and click **Install**.
5. Open the **Configuration** tab, fill in all options (see below), and click **Save**.
6. Start the add-on. Check the **Log** tab to confirm it printed `listening on ...`.

## Options Reference

| Option | Type | Description |
|---|---|---|
| `controller_url` | string | Full URL of your UniFi OS controller, e.g. `https://192.168.1.1` |
| `controller_site` | string | UniFi site name (default: `default`) |
| `controller_username` | string | Local admin user created for this tool |
| `controller_password` | password | Password for the controller user |
| `controller_insecure_tls` | bool | Skip TLS verification (needed for self-signed certs) |
| `auth_token` | password | Bearer token for API authentication — store in `secrets.yaml` |
| `presets` | list | Named presets; each entry has `name`, `switch`, `port`, `profile` |

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
unifi_switcher_token: "your-long-random-bearer-token-here"
```

### 2. Add `rest_command` entries

In `configuration.yaml`:
```yaml
rest_command:
  unifi_presets_list:
    url: "http://a0d7b954-unifi-port-profile-switcher:8099/presets"
    method: GET
    headers:
      Authorization: "Bearer !secret unifi_switcher_token"

  unifi_preset_status:
    url: "http://a0d7b954-unifi-port-profile-switcher:8099/presets/{{ preset }}/status"
    method: GET
    headers:
      Authorization: "Bearer !secret unifi_switcher_token"

  unifi_preset_apply:
    url: "http://a0d7b954-unifi-port-profile-switcher:8099/presets/{{ preset }}/apply"
    method: POST
    headers:
      Authorization: "Bearer !secret unifi_switcher_token"
```

> **Note:** The internal hostname `a0d7b954-unifi-port-profile-switcher` is derived from the add-on slug. HA Supervisor creates a DNS entry for each running add-on.

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

## Upgrading from 2.x

Version 3.0.0 switches the container base image from Debian to Alpine (via the official HA base image) and tightens the options schema. Supervisor will prompt you to **reconfigure** after installing the update.

**What you need to do:**
1. Open the **Configuration** tab after the update installs.
2. Review your settings — Supervisor will highlight any values that no longer match the stricter schema.
3. Click **Save** and start the add-on.

**Schema changes that may require attention:**

| Option | Old | New | Notes |
|---|---|---|---|
| `controller_url` | `str` | `url(http,https)` | Must start with `http://` or `https://` |
| `controller_site` | `str` | `match(^[A-Za-z0-9_-]+$)?` | Alnum, dash, underscore only |
| `presets[].name` | `str` | `match(^[a-z0-9-]+$)` | Lowercase alnum and dash only |
| `presets[].port` | `int` | `int(1,52)` | Must be between 1 and 52 |

**What stays the same:**
- Internal hostname: `a0d7b954-unifi-port-profile-switcher`
- Exposed port: `8099/tcp`
- Bearer-token authentication model
- All HTTP endpoints (`/healthz`, `/presets`, `/presets/<name>/apply`, `/presets/<name>/status`)
- Existing `rest_command` and Lovelace wiring requires no changes

## Troubleshooting

- **Add-on not appearing in store**: Make sure the repository URL was added exactly as shown. Refresh the store.
- **`listening on ...` not in logs**: Check the **Configuration** tab — `auth_token` must not be empty.
- **401 from HA automation**: Confirm the bearer token in `secrets.yaml` matches the `auth_token` option exactly.
- **502 from apply**: The add-on cannot reach the UniFi controller. Check `controller_url` and credentials; if using a self-signed cert, enable `controller_insecure_tls`.
