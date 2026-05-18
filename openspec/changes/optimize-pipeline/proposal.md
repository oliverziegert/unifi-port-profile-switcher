## Why

The CI workflows in `.github/workflows/` were lifted from the Home Assistant `example-app` template and never tailored to this repository. The result is a pipeline that treats the project as a generic multi-app HA repository when it is in fact a **Go module that also ships one HA addon**. Two consequences follow: (1) the pipelines do not exercise the Go code at all — `go test`, `go vet`, and `golangci-lint` are documented in `README.md` but never run in CI, so regressions in the CLI/HTTP layers are caught only by humans, and (2) the addon side carries scaffolding for problems this repo does not have (filtering non-addon directories that match `config.*`, an `io.hass.type=app` label that mis-labels the addon, no Dockerfile lint, no Go module cache, and no release path for the standalone CLI binary the README documents).

## What Changes

- Add a **Go CI workflow** (`go.yaml`) that runs on push/PR to `main` and on a nightly schedule. It will:
  - Cache Go modules and the build cache.
  - Run `go vet ./...`, `go build ./...`, and `go test ./... -race -coverprofile=...` against the supported Go toolchain pinned in `go.mod`.
  - Run `golangci-lint` with a project-local `.golangci.yaml` (created in this change with a small, opinionated rule set: `govet`, `staticcheck`, `errcheck`, `gofmt`, `goimports`, `ineffassign`, `unused`, `revive`).
  - Upload the coverage profile as a workflow artifact for inspection.
- **Optimize the existing addon pipelines** (`builder.yaml`, `build-app.yaml`, `lint.yaml`):
  - Add **buildx caching** (registry cache or GHA cache) for the multi-arch addon image build so unchanged dependency layers are not rebuilt on every PR.
  - Fix the addon image labels to use the **correct HA convention** (`io.hass.type=addon`, not `app`), and add the standard OCI labels (`org.opencontainers.image.source`, `.revision`, `.created`) sourced from the workflow context rather than hand-written in the Dockerfile.
  - Add `hadolint` to the lint workflow so Dockerfile regressions are surfaced alongside addon-config regressions.
  - Keep `home-assistant/actions/helpers/find-addons` for forward compatibility with a future second addon (per maintainer decision), but **move the "filter directories without a Dockerfile" step into a single shared composite action** so `builder.yaml` and `lint.yaml` stop duplicating the same `jq`/bash filter logic. **BREAKING** for forks copying the filter snippet by hand.
  - Replace the `MONITORED_FILES` regex string with a path-filter that includes `**/go.mod`, `**/go.sum`, `**/*.go`, `**/translations/**`, and the existing addon files, so a change to a Go source file inside `unifi-port-profile-switcher/` triggers a rebuild of the addon image (today it does not, because `*.go` is not in the monitored set).
- Add a **release workflow** (`release.yaml`) that triggers on tag push (`v*`) and:
  - Cross-compiles the Go CLI for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`.
  - Generates SHA-256 checksums.
  - Attaches the binaries and checksums to the GitHub Release for that tag.
  - Does **not** publish to any package registry; the addon image is still the only registry artifact.
- Tighten **workflow permissions** to the principle of least privilege: every workflow declares its own minimum `permissions:` block at the workflow level, and step-level `permissions:` are dropped where the workflow-level grant is sufficient.
- Pin the dev-container builder image and golangci-lint to **digest-pinned versions tracked by Renovate** (the existing Renovate config already covers `uses:` references; this change extends the pattern to inline `image:` references in workflows).

## Capabilities

### New Capabilities

- `ci-go-checks`: continuous-integration checks specific to the Go module — vet, build, race-tested unit tests with coverage, and a lint pass over the Go source tree.
- `ci-cli-release`: tag-triggered packaging of the standalone Go CLI binary as GitHub Release assets across the supported OS/arch matrix, with checksums.

### Modified Capabilities

<!-- The existing addon builder/lint workflows have no spec today and were inherited from the HA example-app template, so they are introduced as new capabilities below rather than as deltas. -->

- `ci-addon-image`: existing multi-arch addon image build and publish. **New capability spec** documenting the optimized behaviour (buildx cache, corrected HA labels, Go-source change detection, shared addon-discovery filter).
- `ci-addon-lint`: existing addon-config lint workflow. **New capability spec** documenting the optimized behaviour (Dockerfile linting added, shared addon-discovery filter, scheduled run cadence).

## Impact

- Workflows: `.github/workflows/build-app.yaml`, `.github/workflows/builder.yaml`, `.github/workflows/lint.yaml` rewritten in place. New files `.github/workflows/go.yaml`, `.github/workflows/release.yaml`, and a composite action at `.github/actions/find-addons-filtered/action.yaml`.
- Config: new `.golangci.yaml` at repo root configuring the agreed-upon linter set. New entries in `renovate.json` if needed to track the linter version and any new `uses:` references.
- Dockerfile: `unifi-port-profile-switcher/Dockerfile` updated to drop labels that are now injected from the workflow, leaving only the static labels (title, description, license).
- Docs: `README.md` "Develop" section extended to mention `golangci-lint`. `unifi-port-profile-switcher/CHANGELOG.md` gets an entry noting that the addon image labels are now HA-compliant.
- Compatibility:
  - Existing addon image consumers (Home Assistant supervisor) are unaffected — image name, tag scheme, and architecture matrix are unchanged.
  - **BREAKING** for downstream forks that depend on the inline filter snippet in the workflows (they will need to switch to the new composite action or copy it).
  - Net new GitHub Actions minutes per PR: roughly +2 min for Go test/vet/lint (with module cache), +0 net for the addon build (cache offsets the new label/path-filter work).
