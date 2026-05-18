## ADDED Requirements

### Requirement: Go vet, build, and race-tested unit tests on every PR and push to main

The CI system SHALL run `go vet ./...`, `go build ./...`, and `go test ./... -race -coverprofile=coverage.out` against the Go module at `unifi-port-profile-switcher/` on every pull request targeting `main` and on every push to `main`. The toolchain version SHALL be the version pinned in `unifi-port-profile-switcher/go.mod`.

The job SHALL fail if any of the three commands exits non-zero, and the failure SHALL surface as a distinct PR status check separate from the addon-image and addon-lint checks.

#### Scenario: PR with a passing test suite produces a green Go check

- **WHEN** a pull request targets `main` and `go vet ./...`, `go build ./...`, and `go test ./... -race` all exit zero against the addon module
- **THEN** the `go / test` status check on the PR SHALL be green
- **AND** the coverage profile SHALL be uploaded as a workflow artefact named `coverage`

#### Scenario: PR with a failing test fails the Go check independently of the addon-image build

- **WHEN** a pull request targets `main` and at least one `go test` invocation exits non-zero
- **THEN** the `go / test` status check SHALL be red
- **AND** the addon-image build SHALL still run and report its own status independently

#### Scenario: PR with a `go vet` finding fails the Go check before tests are reported

- **WHEN** a pull request targets `main` and `go vet ./...` exits non-zero
- **THEN** the `go / vet-build` status check SHALL be red
- **AND** the failure message SHALL include the vet diagnostic verbatim

### Requirement: Go module and build cache reused across runs

The Go CI workflow SHALL configure `actions/setup-go` (or equivalent) with the `cache-dependency-path` pointing at `unifi-port-profile-switcher/go.sum`. On a cache hit, the workflow SHALL NOT re-download module sources before running checks.

#### Scenario: Second consecutive run on the same `go.sum` reuses the module cache

- **WHEN** two consecutive workflow runs execute against the same `unifi-port-profile-switcher/go.sum`
- **THEN** the second run's `setup-go` step SHALL report a cache hit
- **AND** no `go: downloading` lines SHALL appear in the test or build logs

#### Scenario: A change to go.sum invalidates the cache

- **WHEN** a PR modifies `unifi-port-profile-switcher/go.sum`
- **THEN** the workflow run SHALL report a cache miss
- **AND** the new cache key SHALL be written for subsequent runs

### Requirement: Lint pass with a curated rule set

The CI system SHALL run `golangci-lint` against the addon module with an explicit, repository-tracked configuration file (`.golangci.yaml` at the repository root). The enabled linters SHALL be exactly: `govet`, `staticcheck`, `errcheck`, `gofmt`, `goimports`, `ineffassign`, `unused`, `revive`. New default linters introduced by future `golangci-lint` versions SHALL NOT silently activate.

The lint job SHALL fail if any enabled linter reports a finding.

#### Scenario: Code with an unchecked error fails the lint check

- **WHEN** a PR introduces a call to a function returning `error` whose result is not assigned, returned, or checked
- **THEN** the `go / lint` status check SHALL be red
- **AND** the `errcheck` finding SHALL appear in the workflow log

#### Scenario: A `golangci-lint` version bump that adds new default linters does not change repo CI behaviour

- **WHEN** `golangci-lint` is upgraded to a version that enables additional linters by default
- **THEN** only the eight explicitly-enabled linters in `.golangci.yaml` SHALL run
- **AND** existing PRs SHALL NOT begin failing on new lint categories without an explicit `.golangci.yaml` edit

### Requirement: Nightly scheduled run

The Go CI workflow SHALL run on a daily schedule (cron) in addition to PR and push triggers, so that toolchain-related regressions (new `golangci-lint` version pulled by Renovate, new Go minor release) surface independently of code changes.

#### Scenario: Scheduled run on a quiet day still runs the full Go check matrix

- **WHEN** the workflow's scheduled trigger fires on a day with no commits to `main`
- **THEN** the full `vet-build`, `test`, and `lint` jobs SHALL execute against the current `HEAD` of `main`
- **AND** failures SHALL be reported via the workflow's normal failure-notification path