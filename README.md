# UniFi Port Profile Switcher

A small Go CLI that flips a UniFi switch port between named profile presets, by
talking to a self-hosted UniFi OS controller's internal REST API.

Built to solve a specific problem: a docking station shared between a personal
and a work laptop. Because the dock presents one MAC, the controller can't
auto-assign different VLANs — but a one-shot CLI can.

## Install

```sh
go build -o unifi-port-profile-switcher .
sudo install -m 0755 ./unifi-port-profile-switcher /usr/local/bin/
```

## Configure

Create a dedicated local admin user on the UniFi Network app (e.g.
`port-switcher`) — used only by this tool. Then write the config file:

```sh
sudo mkdir -p /etc/unifi-port-profile-switcher
sudo install -m 0600 /dev/null /etc/unifi-port-profile-switcher/config.yaml
sudo $EDITOR /etc/unifi-port-profile-switcher/config.yaml
```

Example:

```yaml
controller:
  url: https://192.168.1.1
  site: default
  username: port-switcher
  password: REPLACE_ME
  insecure_tls: true        # self-signed cert on UDM/UDR

presets:
  work-laptop:
    switch: "Office USW-24" # device name OR MAC ("aa:bb:cc:dd:ee:ff")
    port: 5
    profile: "Work VLAN"
  personal-laptop:
    switch: "Office USW-24"
    port: 5
    profile: "Personal VLAN"
```

The tool refuses to load a config that is group- or world-readable. Keep it `0600`.

The tool will also look at `$XDG_CONFIG_HOME/unifi-port-profile-switcher/config.yaml`
(or `~/.config/unifi-port-profile-switcher/config.yaml`) before falling back to
`/etc/...`. Override with `--config /path/to/config.yaml`.

## Usage

```sh
unifi-port-profile-switcher list                   # list presets
unifi-port-profile-switcher status work-laptop     # current vs target
unifi-port-profile-switcher --dry-run work-laptop  # show change, do not apply
unifi-port-profile-switcher work-laptop            # flip the port
```

Apply is idempotent: running it twice in a row is a no-op the second time. Other
ports' overrides on the same switch are preserved.

### Flags

- `--config PATH` — override config file path
- `--dry-run` — compute the change without writing
- `--json` — emit a single JSON object on stdout (machine-readable)
- `-v`, `--verbose` — debug logging on stderr

### Exit codes

| code | meaning |
|---:|---|
| 0 | success or no-op |
| 1 | generic / unexpected error |
| 2 | preset not found in config |
| 3 | controller authentication failure |
| 4 | switch, port, or profile not found |
| 5 | controller API error (non-2xx or `meta.rc != "ok"`) |

## Why not API keys?

Ubiquiti's local API keys authorize the `/proxy/network/integration/v1/*` surface,
which does **not** include port-override endpoints. The legacy
`/proxy/network/api/s/<site>/rest/device/<id>` path (used by the web UI) still
requires session-cookie + `X-CSRF-Token` auth. This tool therefore uses a local
admin username/password.

## Home Assistant add-on

The recommended way to use this tool on Home Assistant OS is via the **Home Assistant add-on**, which ships the same binary as an HTTP API service that HA can call from automations and dashboard buttons.

Full documentation: [unifi-port-profile-switcher/DOCS.md](unifi-port-profile-switcher/DOCS.md)

### Quick install

1. In HA go to **Settings → Add-ons → Add-on Store → Repositories** and add:
   ```
   https://github.com/oliverziegert/unifi-port-profile-switcher
   ```
2. Install the **UniFi Port Profile Switcher** add-on from the store.
3. Configure controller credentials, presets, and a bearer token in the **Configuration** tab.
4. Start the add-on. The HTTP API is now reachable inside HA at port `8099`.

### Phase 2 verification

Confirm the add-on is talking to the controller:

```sh
# 1. Healthcheck (no auth)
curl http://<ha-ip>:8099/healthz
# → {"ok":true}

# 2. List presets (bearer token required)
curl -H "Authorization: Bearer <token>" http://<ha-ip>:8099/presets

# 3. Dry-run apply — reads controller state, no write
curl -X POST -H "Authorization: Bearer <token>" \
  "http://<ha-ip>:8099/presets/work-laptop/apply?dry_run=1"
# → {"changed":false,"dry_run":true,...}

# 4. Real apply
curl -X POST -H "Authorization: Bearer <token>" \
  http://<ha-ip>:8099/presets/work-laptop/apply
# → {"changed":true,...}

# 5. Idempotent re-run (same apply again)
# → {"changed":false,...}
```

## Dependency updates

This repository uses the [Renovate GitHub App](https://github.com/apps/renovate) to keep dependencies up to date across Go modules, GitHub Actions, and the add-on Dockerfile base image.

**How it works:**

- Minor and patch updates are grouped into a single weekly PR per ecosystem (Go modules, GitHub Actions, Docker), opening Monday mornings (Europe/Berlin).
- Vulnerability-driven updates open immediately as individual PRs, labelled `security`.
- Major version bumps open as individual PRs immediately, outside any grouping, so they can be reviewed deliberately.
- Auto-merge is intentionally **disabled** — every Renovate PR must be merged by a human reviewer.

**Opting a dependency out:** add a `packageRules` entry to `renovate.json`:

```json
{
  "matchPackageNames": ["example/package"],
  "enabled": false
}
```

**Note for contributors:** GitHub Actions `uses:` references are pinned to a 40-character commit SHA with a trailing version comment (e.g., `uses: actions/checkout@abc123 # v4.2.0`). Do not remove or "clean up" these comments — Renovate relies on them to track the current version.

## Develop

```sh
go test ./...
go vet ./...
go build ./...
```
