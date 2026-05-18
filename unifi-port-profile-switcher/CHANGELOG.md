<!-- https://developers.home-assistant.io/docs/apps/presentation#keeping-a-changelog -->

## Unreleased

### Changed
- Addon image now carries `io.hass.type=addon` (was `io.hass.type=app`). The Home Assistant supervisor keys on the addon manifest rather than this label, so there is no user-visible behaviour change; the new label simply matches the HA convention.
- Addon image carries OCI provenance labels injected from the CI workflow context: `org.opencontainers.image.revision` (commit SHA), `org.opencontainers.image.version` (addon version), `org.opencontainers.image.created` (build timestamp, RFC 3339), `org.opencontainers.image.source` (repository URL), plus `title`, `description`, and `licenses`. The corresponding `LABEL` block was removed from the Dockerfile.

## 0.0.1

### Added
- `GET /ports/{switch}/{port}/active` — read-only endpoint that returns which configured preset (if any) is currently active on a switch+port, plus the port's current profile. Dashboards can now use **one** `rest` sensor per port instead of one per preset. Existing `/presets/...` endpoints are unchanged. See `DOCS.md` → "Highlight the active preset" for the updated `configuration.yaml` and Lovelace examples.

## 0.0.1

> **Reconfigure required on upgrade.** The options schema is stricter in 3.0.0.
> After installing, open the **Configuration** tab, review your settings, and click **Save**.
> Your existing `rest_command` entries, bearer-token, and exposed port are **unchanged**.

### Added
- Runs on the official `ghcr.io/home-assistant/{arch}-base` image (Alpine + S6-overlay v3) instead of Debian — smaller image, S6 supervision built in.
- `bashio`-based service entry script (`rootfs/etc/services.d/`) with structured log output visible in the Supervisor **Log** tab.
- S6 supervision: if `serve` exits, S6 restarts it; persistent failures halt the container and trigger Supervisor's watchdog for a full restart.
- `apparmor: true` with a custom `apparmor.txt` profile scoped to the binary's actual filesystem and network needs.
- `watchdog: http://[HOST]:[PORT:8099]/healthz` — Supervisor probes and auto-restarts a wedged container.
- `homeassistant: "2025.4"` minimum Core version.
- `backup: cold` — Supervisor stops the add-on before snapshotting.
- `panel_icon: mdi:lan-connect`.
- Signed multi-arch images (`amd64`, `aarch64`) via `home-assistant/builder` with codenotary notarization.
- `build.yaml` with per-arch base image pins managed by Renovate.

### Changed
- `controller_url` schema: `str` → `url(http,https)`.
- `controller_site` schema: `str` → `match(^[A-Za-z0-9_-]+$)?`.
- `presets[].name` schema: `str` → `match(^[a-z0-9-]+$)`.
- `presets[].port` schema: `int` → `int(1,52)`.

### Removed
- Debian runtime base image and `jq` dependency.
- Top-level `run.sh` (replaced by `rootfs/etc/services.d/` S6 layout).