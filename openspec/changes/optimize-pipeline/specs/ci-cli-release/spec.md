## ADDED Requirements

### Requirement: Tag-triggered cross-compiled CLI binaries

The CI system SHALL run a release workflow on every push of a tag matching `v*` (semver tags). The workflow SHALL cross-compile the standalone CLI binary for the following OS/architecture combinations: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`. Builds SHALL use `CGO_ENABLED=0`.

#### Scenario: Pushing a `v0.0.5` tag produces four CLI binaries on the GitHub Release

- **WHEN** a tag matching `v[0-9]+.[0-9]+.[0-9]+` (or a semver pre-release form) is pushed to the repository
- **THEN** the release workflow SHALL run and SHALL upload exactly four binaries to the corresponding GitHub Release: `unifi-port-profile-switcher_<version>_linux_amd64`, `_linux_arm64`, `_darwin_amd64`, `_darwin_arm64`
- **AND** each binary's filename SHALL include the tag's version string verbatim (without the `v` prefix)

#### Scenario: A non-semver tag does not trigger a release build

- **WHEN** a tag not matching the `v*` pattern is pushed (for example, `nightly` or `internal-2026-05-17`)
- **THEN** the release workflow SHALL NOT run
- **AND** no release artefacts SHALL be created

### Requirement: SHA-256 checksum file accompanies the release

The release workflow SHALL generate a single `SHA256SUMS` file containing the SHA-256 checksum of each uploaded binary, one per line, in the format produced by `sha256sum`. The `SHA256SUMS` file SHALL be uploaded alongside the binaries on the same GitHub Release.

#### Scenario: Release assets include a verifiable checksum file

- **WHEN** the release workflow completes successfully for a `v*` tag
- **THEN** the GitHub Release SHALL contain a `SHA256SUMS` file
- **AND** running `sha256sum -c SHA256SUMS` against the four downloaded binaries in the same directory SHALL report `OK` for each

### Requirement: Release workflow does not modify the addon image pipeline

The release workflow SHALL NOT push to any container registry, SHALL NOT alter the `ghcr.io/oliverziegert/unifi-port-profile-switcher` image, and SHALL NOT depend on or be depended on by the addon image build/publish workflow.

#### Scenario: A tag push that triggers a release does not republish the addon image from the release workflow

- **WHEN** a `v*` tag is pushed and the release workflow runs to completion
- **THEN** the release workflow's job log SHALL contain no docker, buildx, or registry-push commands
- **AND** the addon image at `ghcr.io/oliverziegert/unifi-port-profile-switcher` SHALL be unchanged by the release workflow (any concurrent addon-image update SHALL come from the existing addon build workflow, not this one)

### Requirement: Least-privilege permissions

The release workflow SHALL declare `permissions: contents: write` at the workflow level and SHALL NOT request any other permission. Step-level `permissions:` blocks SHALL NOT be used to broaden the workflow-level grant.

#### Scenario: Release workflow's effective permissions are exactly `contents: write`

- **WHEN** the release workflow runs
- **THEN** the GITHUB_TOKEN scope reported in the workflow log SHALL be exactly `contents: write`
- **AND** no other scope (`packages`, `id-token`, etc.) SHALL be granted
