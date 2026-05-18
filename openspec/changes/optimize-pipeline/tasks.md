## 1. Shared composite action for addon discovery

- [x] 1.1 Create `.github/actions/find-addons-filtered/action.yaml` as a composite action that runs `actions/checkout` (if not already done in the caller), invokes `home-assistant/actions/helpers/find-addons@<pinned-sha>`, and applies the existing Dockerfile-presence filter via a single bash step.
- [x] 1.2 Define outputs `addons` (JSON array of all addon paths) and `changed_apps` (JSON array filtered against a caller-supplied `changed_files` input). When no `changed_files` input is provided, `changed_apps` equals `addons`.
- [x] 1.3 Include input `monitored_globs` (multi-line string, default `config.{json,yaml,yml}`, `Dockerfile`, `rootfs/**`, `translations/**`, `**/*.go`, `go.mod`, `go.sum`) so the path-filter list lives next to the discovery logic.
- [x] 1.4 Honour the workflow-rebuild escape hatch: when `changed_files` contains a `.github/workflows/(builder|build-app)\\.yaml` entry, emit every addon in `changed_apps` regardless of glob matching.
- [x] 1.5 Add a `README.md` next to the composite action describing inputs/outputs and the rationale (one paragraph, no marketing prose).

## 2. Root-level .golangci.yaml

- [x] 2.1 Add `.golangci.yaml` at the repository root enabling exactly: `govet`, `staticcheck`, `errcheck`, `gofmt`, `goimports`, `ineffassign`, `unused`, `revive`. (Implemented in golangci-lint v2 schema: `linters.default: none` with explicit `enable:` list; `gofmt` and `goimports` are configured as v2 `formatters` so future defaults still cannot silently activate.)
- [x] 2.2 Configure `goimports.local-prefixes: github.com/oliverziegert/unifi-port-profile-switcher` so internal imports group correctly.
- [x] 2.3 Set the run `timeout: 5m` and `tests: true` so test files are also linted.
- [x] 2.4 Verify locally: from `unifi-port-profile-switcher/`, ran `golangci-lint run --config ../.golangci.yaml ./...` — 10 findings resolved (errcheck on `fmt.Fprint*` stdout/stderr writes and `defer res.Body.Close()`, revive package-comment on `main.go`); linter now reports `0 issues.`

## 3. Go CI workflow

- [x] 3.1 Create `.github/workflows/go.yaml` triggered on `pull_request: branches: [main]`, `push: branches: [main]`, and `schedule: - cron: "0 3 * * *"` (UTC; off-peak vs the existing lint cron at midnight UTC).
- [x] 3.2 Declare workflow-level `permissions: contents: read`. No other scopes.
- [x] 3.3 Define three parallel jobs (`vet-build`, `test`, `lint`), each on `ubuntu-latest`, each with steps: `actions/checkout@<pinned>`, `actions/setup-go@<pinned>` with `cache-dependency-path: unifi-port-profile-switcher/go.sum` and `go-version-file: unifi-port-profile-switcher/go.mod`.
- [x] 3.4 `vet-build` job: `working-directory: unifi-port-profile-switcher`, run `go vet ./...` then `go build ./...`.
- [x] 3.5 `test` job: `working-directory: unifi-port-profile-switcher`, run `go test ./... -race -coverprofile=coverage.out -covermode=atomic`, then `actions/upload-artifact@<pinned>` uploading `coverage.out` as artifact name `coverage`.
- [x] 3.6 `lint` job: use `golangci/golangci-lint-action@<pinned-sha>` with `working-directory: unifi-port-profile-switcher` and `args: --config=../.golangci.yaml`. Do not pass an explicit version; let Renovate pin it via the action SHA.
- [x] 3.7 Pin every `uses:` reference to a 40-character SHA with a trailing `# vX.Y.Z` comment, matching the existing repo convention.

## 4. Rewrite addon image workflows

- [x] 4.1 Rewrite `.github/workflows/builder.yaml`: declare workflow-level `permissions: contents: read`; drop the env `MONITORED_FILES` regex; replace the manual filter step with a call to `./.github/actions/find-addons-filtered` passing the output of `tj-actions/changed-files` and using the action's default `monitored_globs`.
- [x] 4.2 In `builder.yaml`'s matrix call to `build-app.yaml`, leave the `app:` and `publish:` inputs as today; nothing else needs to change at the caller.
- [x] 4.3 Rewrite `.github/workflows/build-app.yaml`: workflow-level `permissions: contents: read, id-token: write, packages: write`; drop redundant step-level `permissions:` blocks if any. (Existing duplicates were job-level rather than step-level; both removed.)
- [x] 4.4 Update the `home-assistant/builder/actions/build-image` step. Verified `actions/build-image` does NOT expose `cache-from`/`cache-to` pass-through inputs — it manages buildx caches internally via `cache-gha` (default true, scoped by arch) plus a registry-cache read of `<image>:<cache-image-tag>` (defaults to `:latest`). Per the task's escape clause, scoped to the action's built-in `type=gha` cache and documented the trade-off in `build-app.yaml`'s top comment; the `:buildcache-<arch>` registry-cache scheme from the design is not wired up.
- [x] 4.5 Replace the `io.hass.type=app` label with `io.hass.type=addon`. Keep `io.hass.name`, `io.hass.description`, and (when present) `io.hass.url` populated from the `prepare` job's outputs.
- [x] 4.6 Add OCI provenance labels. The `actions/build-image` action already auto-injects `org.opencontainers.image.created` (from `date --rfc-3339=seconds --utc`), `.source` (from `${GITHUB_REPOSITORY}`), and `.version` (from the `version:` input), so the only new label written from this workflow is `org.opencontainers.image.revision=${{ github.sha }}`. Also added `org.opencontainers.image.title`, `.description`, and `.licenses` from the workflow context so the Dockerfile no longer hand-writes them.
- [x] 4.7 Remove from `unifi-port-profile-switcher/Dockerfile` the labels now injected from the workflow (`org.opencontainers.image.source`, `org.opencontainers.image.title`, `org.opencontainers.image.description`, `org.opencontainers.image.licenses`). Replaced the LABEL block with a short comment pointing readers to `build-app.yaml`.
- [x] 4.8 Confirm tag-write convention is unchanged: per-arch image tags (`<version>` + `latest`) and the multi-arch manifest both published with the same two tags.

## 5. Rewrite addon lint workflow

- [x] 5.1 Rewrite `.github/workflows/lint.yaml`: declare workflow-level `permissions: contents: read`; drop the inline filter step in `find` and call `./.github/actions/find-addons-filtered` for the addon list.
- [x] 5.2 Add a second matrix job `hadolint` that runs `hadolint/hadolint-action@<pinned-sha>` against each discovered addon's `Dockerfile`. Use the same matrix source as the `lint` job.
- [x] 5.3 Keep the existing schedule trigger (`cron: "0 0 * * *"`) and keep the `frenck/action-addon-linter` invocation otherwise unchanged.

## 6. CLI release workflow

- [x] 6.1 Create `.github/workflows/release.yaml` triggered on `push: tags: ['v*']`.
- [x] 6.2 Declare workflow-level `permissions: contents: write`. No other scopes.
- [x] 6.3 Per-matrix `build` job and an `aggregate` job both run on `ubuntu-latest`, with the matrix over `{os: [linux, darwin]} × {arch: [amd64, arm64]}` (four combinations). Split into two jobs because `softprops/action-gh-release` needs all four binaries at once for a single release publish + checksum file.
- [x] 6.4 Steps per matrix entry: checkout, setup-go (using the addon module's `go.mod` for version), then `GOOS=${{ matrix.os }} GOARCH=${{ matrix.arch }} CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=${GITHUB_REF_NAME#v}" -o unifi-port-profile-switcher_${GITHUB_REF_NAME#v}_${{ matrix.os }}_${{ matrix.arch }} .` from `unifi-port-profile-switcher/`, then upload the binary as an artifact for the aggregate job.
- [x] 6.5 Added the `aggregate` job: `needs: build`, downloads all four binaries via `actions/download-artifact` with `merge-multiple: true`, runs `sha256sum unifi-port-profile-switcher_*_*_* > SHA256SUMS`, then invokes `softprops/action-gh-release@<pinned-sha>` with `files:` covering all four binaries plus `SHA256SUMS`, `draft: false`, `prerelease: contains(github.ref_name, '-')`.
- [x] 6.6 `main.version` did not exist; added `var version = "dev"` at package level in `main.go`, plus a tiny `--version` flag handler so the symbol is referenced (avoids a `staticcheck`/`unused` linter complaint while keeping the change strictly additive).

## 7. Documentation

- [x] 7.1 Add a "Lint" subsection under the README's "Develop" section listing the eight enabled linters and the `golangci-lint run --config ../.golangci.yaml ./...` invocation.
- [x] 7.2 Add a "Releases" subsection under the README's existing "Install" section pointing to the GitHub Releases page for users who want a prebuilt CLI binary, with a short note that the binaries are unsigned (darwin users will need to `xattr -d com.apple.quarantine`).
- [x] 7.3 Add an entry to `unifi-port-profile-switcher/CHANGELOG.md` under the next unreleased version noting (a) the addon image now carries `io.hass.type=addon` (was `app`), and (b) the new OCI provenance labels.
- [x] 7.4 Update the root `README.md` "Dependency updates" subsection to mention the composite action is also Renovate-tracked via its inner `uses:` references.

## 8. Verification

- [ ] 8.1 Open a draft PR containing only the new composite action and `.golangci.yaml`. Confirm the existing pipelines stay green (the composite is not yet used).
- [ ] 8.2 Land the Go CI workflow in a second draft PR. Confirm one green run on `main` after merge, then confirm the nightly cron fires the next day and produces a green run.
- [ ] 8.3 Land the rewritten addon workflows in a third PR. On that PR, push:
    1. A docs-only commit → confirm the addon build is skipped.
    2. A Go-source commit inside `unifi-port-profile-switcher/internal/` → confirm the addon build runs for every arch.
    3. After merge to `main`, run `docker inspect ghcr.io/oliverziegert/unifi-port-profile-switcher:<version>` and verify `io.hass.type=addon`, `org.opencontainers.image.revision`, `.version`, and `.created` labels are present and correct.
- [ ] 8.4 Land the release workflow in a fourth PR. After merge, create a `v0.0.0-rc.0` pre-release tag and verify the GitHub Release receives four binaries plus a `SHA256SUMS` file, and that `sha256sum -c SHA256SUMS` validates against the downloaded binaries.
- [ ] 8.5 After all four PRs are merged, delete any unused workflow files or steps that are now dead code, and confirm no remaining workflow file contains the inline jq/bash addon-filter duplication.
- [ ] 8.6 Update the `optimize-pipeline` OpenSpec change's status and run `/opsx:archive optimize-pipeline` to move the change into `openspec/changes/archive/`.
