## Context

The repository is a single Go module (`github.com/oliverziegert/unifi-port-profile-switcher`, Go 1.26) that produces two consumer-facing artefacts:

1. A standalone CLI binary, installed via `go build` as documented in `README.md`.
2. A Home Assistant addon at `unifi-port-profile-switcher/` that wraps the same code as a long-running HTTP service. The addon ships as a multi-arch container image at `ghcr.io/oliverziegert/unifi-port-profile-switcher`.

The current workflows were copied verbatim from the Home Assistant `example-app` template:

- `builder.yaml` watches `config.json config.yaml config.yml Dockerfile rootfs` for changes and rebuilds matching "apps". A separate jq/bash filter drops directories that happen to match `config.*` but lack a `Dockerfile` (this exists because the find-addons helper greedily picks up `.serena/`, `openspec/`, etc., all of which have `config.yaml`).
- `build-app.yaml` is a reusable workflow that drives `home-assistant/builder/actions/{prepare-multi-arch-matrix, build-image, publish-multi-arch-manifest}`, with three native-arch jobs in parallel (amd64, aarch64) plus a manifest job.
- `lint.yaml` runs `frenck/action-addon-linter` for each discovered addon directory, with the same jq/bash filter inline.

No workflow runs `go test`, `go vet`, `go build`, or `golangci-lint`. The repo's `go.mod` pin (`go 1.26`) is therefore validated only on the maintainer's workstation.

Renovate is already configured to pin GitHub Actions `uses:` references to commit SHAs with a version comment; this convention must be preserved by any new workflow.

The maintainer has confirmed three constraints for this change (recorded in `proposal.md`):

1. Add Go CI (test + vet + golangci-lint, with race detector).
2. **Keep** `home-assistant/actions/helpers/find-addons` (do not hardcode the single addon path) so a future second addon can be added without a workflow rewrite.
3. Publish standalone CLI binaries as GitHub Release assets on tag push.

## Goals / Non-Goals

**Goals:**
- Close the Go-CI gap: every PR and main-branch push verifies `go vet`, `go build`, race-tested unit tests, and a linter pass before merge.
- Stop duplicating the "find addons, drop the non-Dockerfile dirs" logic across `builder.yaml` and `lint.yaml`. Extract it into one place so future addon-discovery edits happen in a single file.
- Make addon image rebuilds correctly fire on Go source changes (today only `config.*`, `Dockerfile`, and `rootfs/**` trigger a rebuild — a change to `internal/server/server.go` does not).
- Restore the HA-standard `io.hass.type=addon` label and add the conventional OCI provenance labels driven by workflow context so they are accurate per-build.
- Make addon image builds materially faster on subsequent runs via buildx layer caching (Go module download + `go build` step is the dominant cost).
- Provide a clean release path for the CLI binary so users not on Home Assistant have a documented "download and run" option.
- Tighten workflow permissions to the least privilege that lets each workflow function.

**Non-Goals:**
- Migrating away from `home-assistant/builder` to plain `docker buildx`. The HA builder gives correct cross-compilation, sets the right base image per architecture, and is the supported path for HA addons; replacing it would be a much larger change with no user-visible benefit.
- Publishing the CLI to package managers (Homebrew, apt, AUR). The release artefacts are downloadable binaries plus checksums; distribution is a separate future change.
- Code coverage gating (failing the build below X%). Coverage is uploaded as an artefact for inspection; no threshold is enforced in this change.
- Backfilling specs for `/healthz`, `/presets`, and `/presets/.../apply`. The earlier add-port-active-endpoint change deferred those; this change does not touch the application surface.
- Fuzz tests, integration tests against a live UniFi controller, or mutation testing. Out of scope.
- A signed release (cosign, SLSA provenance). Useful, but a separable change.

## Decisions

### One Go workflow, not three

Considered alternatives:
- Separate `test.yaml`, `vet.yaml`, `lint.yaml` workflows — surfaces three checks on the PR but triples the checkout+module-restore cost.
- One `go.yaml` with a single job running `go vet && go build && go test && golangci-lint` sequentially — cheapest, but a vet failure masks downstream test failures from the reviewer.

Chosen: one `go.yaml` workflow with **parallel jobs** (`test`, `vet-build`, `lint`) sharing a single module-cache key. Each job runs a minimal subset and surfaces independently on the PR status page. Setup cost is paid three times in parallel; wall-clock equals the slowest job (`test`), and the reviewer sees which check failed without expanding logs.

### `golangci-lint` rule set

Considered alternatives:
- Defaults only — too noisy, includes `gocyclo` etc. that fire on legitimate code.
- The HA-canonical Go linter list — there is no such thing; HA does not write Go.
- The maintainer's existing IDE config — not committed.

Chosen: a small, opinionated `.golangci.yaml` enabling exactly: `govet`, `staticcheck`, `errcheck`, `gofmt`, `goimports`, `ineffassign`, `unused`, `revive`. Rationale: these eight cover the actual bug categories observed in this codebase's history (unchecked errors, dead code, formatting drift) without flagging style-only patterns the maintainer has accepted. Adding more later is cheap; removing a noisy default after PRs are already failing is expensive.

### Module cache key

`actions/setup-go@v5` provides built-in caching keyed on `go.sum`. The Go module path is `unifi-port-profile-switcher/go.mod` (the addon directory), so the workflow must `cache-dependency-path: unifi-port-profile-switcher/go.sum`. Without that override, `setup-go` looks at the repo root, misses the addon `go.sum`, and reports a cache miss every run.

### Composite action for find-addons-filtered

Considered alternatives:
- Continue inlining the filter in each workflow — current state, two places to change.
- A reusable workflow (`workflow_call`) — too heavyweight, requires its own checkout, doubles the runner-start cost.
- A small shell script in `.github/scripts/find-addons-filtered.sh` invoked by each workflow — cheap, but loses the typed `inputs:`/`outputs:` of a composite action.

Chosen: a composite action at `.github/actions/find-addons-filtered/action.yaml`. It wraps `home-assistant/actions/helpers/find-addons`, applies the Dockerfile filter, and exposes:
- `addons` (output) — JSON array of paths, suitable for `strategy.matrix`.
- `changed_apps` (output) — JSON array filtered against a caller-supplied list of changed files.

`builder.yaml` and `lint.yaml` then call this one action.

### `MONITORED_FILES` becomes a path filter

Today the workflow uses a hand-built regex over the output of `tj-actions/changed-files` to detect whether any addon-relevant file changed. This regex is anchored on `config.json|config.yaml|config.yml|Dockerfile|rootfs` only.

Considered alternatives:
- Extend the regex to add `*.go|go.mod|go.sum` — works, but the regex is already opaque and grows.
- Switch to `dorny/paths-filter` for first-class path filtering — adds a dependency, but expresses intent declaratively.
- Replace the diff check entirely and **always** build the addon on push to main, rely on buildx cache for speed — cheapest to reason about, but burns Actions minutes on docs-only PRs.

Chosen: keep `tj-actions/changed-files` (already in use) and replace the regex with a small explicit list of glob patterns evaluated in shell: `config.{json,yaml,yml}`, `Dockerfile`, `rootfs/**`, `translations/**`, `**/*.go`, `go.mod`, `go.sum`. The composite action accepts this list as an input so the same set lives in one place.

### Addon image labels

The HA addon spec calls for `io.hass.type=addon`. The current pipeline writes `io.hass.type=app` (a Home Assistant *application*, a different concept used for non-supervisor containers). Considered alternatives:
- Leave it — works, but technically wrong and might trip a future stricter linter version.
- Add only the missing `io.hass.type=addon` and leave `app` in place — redundant labels, confusing.

Chosen: replace `app` with `addon`. Add OCI labels driven by the workflow environment: `org.opencontainers.image.revision=${{ github.sha }}`, `org.opencontainers.image.created=${{ timestamp }}`, `org.opencontainers.image.version=${{ version }}`. Remove the duplicate static `source`/`title`/`description`/`licenses` labels from the `Dockerfile` since they are now applied per-build with provenance.

### Buildx caching for the addon image

`home-assistant/builder/actions/build-image` invokes `docker buildx` under the hood and accepts a `cache-from`/`cache-to` pass-through. Considered alternatives:
- `type=gha` (GitHub Actions cache) — simplest, but GHA cache is per-branch by default and evicts aggressively.
- `type=registry,ref=ghcr.io/oliverziegert/unifi-port-profile-switcher:cache-${ARCH}` — durable, survives across branches, but requires a separate tag namespace and registry credentials at cache-write time (already available via `secrets.GITHUB_TOKEN`).
- No cache — current state, every PR rebuilds the Go binary from scratch in the container.

Chosen: registry cache with a `:buildcache-<arch>` tag per architecture. Trade-off: an extra image manifest per arch in the registry; mitigated by GHCR's free unlimited public storage.

### Release workflow shape

Considered alternatives:
- `goreleaser` — feature-rich, but introduces a YAML DSL and pulls in changelogs/Homebrew taps that this project does not want.
- Hand-rolled `go build` matrix + `gh release upload` — minimal, no new dependency. Roughly 25 lines of bash.
- `softprops/action-gh-release` — Action wrapper around the GH API, pinned by SHA already used elsewhere.

Chosen: a single `release.yaml` triggered on tag push (`v*`) with one job and a `matrix:` over `{linux,darwin} × {amd64,arm64}`. Each matrix entry builds, computes a SHA-256, and uploads via `softprops/action-gh-release@<pinned-sha>`. The job runs on `ubuntu-latest` for all arches via `GOOS`/`GOARCH` cross-compilation (the CLI has no cgo, confirmed by the existing Dockerfile's `CGO_ENABLED=0`).

### Permissions

Each workflow declares the **minimum** workflow-level permissions:

- `go.yaml`: `contents: read` only.
- `builder.yaml` / `build-app.yaml`: `contents: read, id-token: write, packages: write` (current state, validated as minimal).
- `lint.yaml`: `contents: read` only.
- `release.yaml`: `contents: write` (to create/edit the release).

Step-level grants that duplicate the workflow-level grant are dropped.

## Risks / Trade-offs

- **Composite action introduces an indirection.** Anyone reading `builder.yaml` for the first time has to open a second file to see how addon discovery works. → Mitigation: name the action `find-addons-filtered` (the verb-object describes exactly what it does), add a top-of-file comment in each caller pointing to the action.
- **Registry cache pollution.** Each architecture gets a `:buildcache-<arch>` tag. → Mitigation: the GHCR tag namespace is already cluttered with per-version tags; one cache tag per arch is marginal. Document the convention in the addon `CHANGELOG.md`.
- **`golangci-lint` upgrade friction.** A future linter version can introduce new lints that fail builds. → Mitigation: Renovate pins the action by SHA with a version comment; the maintainer reviews the bump PR like any other dependency. The `.golangci.yaml` `enable:` list is explicit, so new defaults do not silently activate.
- **HA label change is observable.** A consumer (improbable but possible) scraping `io.hass.type=app` will stop matching. → Mitigation: the only documented consumer is the HA supervisor, which keys on the addon manifest, not the image label. Note the change in `unifi-port-profile-switcher/CHANGELOG.md`.
- **Path-filter false negatives.** If the path-filter glob misses a relevant file, a PR can ship an addon image without an actual rebuild. → Mitigation: include a coarse catch-all (`unifi-port-profile-switcher/**`) as a fallback when no specific pattern matches, accepting the occasional extra rebuild over a stale image.
- **Cross-compiled darwin binaries are unsigned.** Users on macOS will hit Gatekeeper. → Mitigation: document in the Release notes; signing is out of scope per non-goals.
- **CI minute cost.** Three parallel Go jobs + addon build per PR. → Mitigation: the module cache plus buildx cache should keep typical PR wall-clock under five minutes; first-build cost is a one-time hit.

## Migration Plan

This is a CI-only change; there is nothing to roll back at runtime.

1. Land the composite action and `.golangci.yaml` first in a single PR — these have no behavioural change to consumers.
2. Land the `go.yaml` workflow second, so the Go CI starts producing signal before the addon workflows are touched. Confirm one full main-branch run before continuing.
3. Land the rewritten `builder.yaml` / `build-app.yaml` / `lint.yaml` together — they share the composite action. Verify on a draft PR that (a) a docs-only change does not trigger an addon build, (b) a Go-source change inside `unifi-port-profile-switcher/` **does** trigger an addon build, (c) the published image has `io.hass.type=addon` and the OCI revision label matches `github.sha`.
4. Land the `release.yaml` workflow last. Test by creating a `v0.0.0-rc.0` tag in a draft state and verifying the four cross-compiled binaries plus a checksum file appear on the corresponding GitHub Release.
5. If any step regresses, revert that step's PR; the steps are independent and the order above keeps the addon publish path healthy at every checkpoint.

## Open Questions

- None at this time. The maintainer's three clarifying answers (Go CI in, keep find-addons, ship CLI binaries) close the previously open scope choices. Lint rule set and cache backend are decided above; if either turns out wrong in practice, swapping them is a one-file edit.
