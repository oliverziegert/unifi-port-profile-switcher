## ADDED Requirements

### Requirement: Multi-arch addon image build on push and PR

The CI system SHALL build the Home Assistant addon image for every architecture listed in the addon's `config.yaml` `arch:` field, on every pull request targeting `main` and on every push to `main`. The image SHALL be published to `ghcr.io/oliverziegert/unifi-port-profile-switcher` only on push to `main` (not on PR builds), in line with the current addon publish convention.

#### Scenario: PR build constructs all architectures but does not publish

- **WHEN** a pull request targets `main` and modifies a file in the addon-build trigger set
- **THEN** the addon image SHALL be built for every architecture in `arch:`
- **AND** the workflow SHALL NOT push any tag to `ghcr.io`

#### Scenario: Push to main builds and publishes all architectures plus the multi-arch manifest

- **WHEN** a commit is pushed to `main` and modifies a file in the addon-build trigger set
- **THEN** the addon image SHALL be built and pushed for every architecture in `arch:`
- **AND** a multi-arch manifest SHALL be published linking those per-arch images under both the version tag and `latest`

### Requirement: Addon-build trigger set covers addon config, Dockerfile, runtime layout, Go source, and workflow itself

The addon-image build SHALL be triggered when a changed file matches any of: `config.{json,yaml,yml}` inside an addon directory, `Dockerfile` inside an addon directory, anything under an addon directory's `rootfs/**` or `translations/**`, anything under the Go module's `**/*.go`, `go.mod`, or `go.sum`, or the workflow files `.github/workflows/builder.yaml` or `.github/workflows/build-app.yaml`. A change to any other file SHALL NOT trigger an addon-image rebuild.

#### Scenario: Editing a Go source file triggers an addon-image rebuild

- **WHEN** a PR modifies only `unifi-port-profile-switcher/internal/server/server.go`
- **THEN** the addon-image build job SHALL run for every architecture in the addon's `arch:` list
- **AND** the resulting image SHALL contain the updated binary

#### Scenario: Editing only documentation does not trigger an addon-image rebuild

- **WHEN** a PR modifies only `README.md`
- **THEN** the addon-image build job SHALL be skipped
- **AND** the workflow status SHALL report the skip reason ("no addon-relevant files changed")

#### Scenario: Editing the addon build workflow itself rebuilds every addon

- **WHEN** a PR modifies `.github/workflows/build-app.yaml`
- **THEN** the addon-image build SHALL run for every discovered addon, regardless of whether files inside that addon's directory changed

### Requirement: Addon image labelled per Home Assistant convention

The published addon image SHALL carry the label `io.hass.type=addon` (not `app`), `io.hass.name`, `io.hass.description`, and `io.hass.url` populated from the addon's `config.yaml`. The image SHALL additionally carry OCI provenance labels populated from the CI environment: `org.opencontainers.image.revision` set to the commit SHA, `org.opencontainers.image.created` set to the build timestamp in RFC 3339 form, and `org.opencontainers.image.version` set to the addon version.

#### Scenario: Inspecting a published image reveals the HA-compliant label set

- **WHEN** a consumer runs `docker inspect ghcr.io/oliverziegert/unifi-port-profile-switcher:<version>` after a successful publish
- **THEN** the labels SHALL include `io.hass.type=addon`
- **AND** the labels SHALL NOT include `io.hass.type=app`
- **AND** `org.opencontainers.image.revision` SHALL match the published commit's SHA
- **AND** `org.opencontainers.image.version` SHALL match the addon's `config.yaml` `version:` field

### Requirement: Buildx layer cache reused across runs

The addon-image build SHALL use a per-architecture registry cache stored under the image's own repository (for example, `ghcr.io/oliverziegert/unifi-port-profile-switcher:buildcache-amd64`). On a cache hit for unchanged dependency layers (Go module download, base-image fetch), those layers SHALL NOT be reconstructed.

#### Scenario: Second consecutive build with no dependency change reuses cached layers

- **WHEN** two consecutive addon-image builds run against the same `go.mod`, `go.sum`, and `Dockerfile` `FROM` lines for a given architecture
- **THEN** the second build SHALL report `CACHED` for the `go mod download` and base-image fetch layers in the buildx log

#### Scenario: A change to go.mod invalidates the relevant cached layer

- **WHEN** a PR modifies `unifi-port-profile-switcher/go.mod`
- **THEN** the `go mod download` layer SHALL NOT be reported as `CACHED` in the next build's log
- **AND** subsequent unrelated layers SHALL still be cached

### Requirement: Shared addon discovery filter

`builder.yaml` and `build-app.yaml` SHALL obtain the list of addon directories by invoking the composite action at `.github/actions/find-addons-filtered`. The inline jq/bash filter that drops non-Dockerfile directories SHALL NOT be duplicated in any workflow file.

#### Scenario: Editing the addon-discovery filter touches exactly one file

- **WHEN** a maintainer needs to change which directories are considered addons (for example, to also require a `config.yaml`)
- **THEN** the edit SHALL be made only in `.github/actions/find-addons-filtered/action.yaml`
- **AND** no edit SHALL be required in `builder.yaml`, `build-app.yaml`, or `lint.yaml` to pick up the new behaviour

### Requirement: Least-privilege workflow permissions

`builder.yaml` SHALL declare workflow-level `permissions: contents: read`. `build-app.yaml` SHALL declare workflow-level `permissions: contents: read, id-token: write, packages: write`. Step-level `permissions:` blocks SHALL NOT broaden these grants.

#### Scenario: PR build runs without write access to repository contents

- **WHEN** a PR triggers `builder.yaml` from a fork
- **THEN** the `GITHUB_TOKEN` granted to the workflow SHALL NOT have `contents: write`
- **AND** no step in the workflow SHALL attempt a write to the repository
