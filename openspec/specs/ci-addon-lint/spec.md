### Requirement: Addon config and Dockerfile lint on PR, push, and nightly

The CI system SHALL lint each discovered addon directory on every pull request targeting `main`, on every push to `main`, and on a daily schedule. The lint pass SHALL include both:

- `frenck/action-addon-linter` against the addon's `config.yaml` and supporting files.
- `hadolint` against the addon's `Dockerfile`.

Each addon directory's lint job SHALL appear as an independent status check, so a failure in one addon does not mask another.

#### Scenario: Addon-config error fails the addon-linter check for that addon only

- **WHEN** a PR introduces an invalid value in `unifi-port-profile-switcher/config.yaml` (for example, a `port:` outside the documented range)
- **THEN** the `lint / addon-linter (unifi-port-profile-switcher)` status check SHALL be red
- **AND** any other discovered addon's lint status check SHALL be unaffected

#### Scenario: Dockerfile lint finding fails the hadolint check

- **WHEN** a PR modifies `unifi-port-profile-switcher/Dockerfile` to use an unpinned base image tag (for example, removes the digest pin)
- **THEN** the `lint / hadolint (unifi-port-profile-switcher)` status check SHALL be red
- **AND** the hadolint finding SHALL appear in the workflow log identifying the line number

#### Scenario: Scheduled run validates against current addon-linter and hadolint versions

- **WHEN** the workflow's scheduled trigger fires
- **THEN** the lint pass SHALL run against the current `HEAD` of `main` with whatever versions of `frenck/action-addon-linter` and `hadolint` are pinned in the workflow at that time
- **AND** failures SHALL be reported via the workflow's normal failure-notification path

### Requirement: Shared addon discovery filter

The lint workflow SHALL obtain the list of addon directories by invoking the composite action at `.github/actions/find-addons-filtered`. The inline jq/bash filter that drops non-Dockerfile directories SHALL NOT be duplicated in `lint.yaml`.

#### Scenario: Discovery filter change is picked up by lint without a lint-workflow edit

- **WHEN** the composite action's filter is updated to additionally require a `config.yaml`
- **THEN** the lint workflow SHALL pick up the new filter on its next run without any edit to `lint.yaml`

### Requirement: Least-privilege workflow permissions

`lint.yaml` SHALL declare workflow-level `permissions: contents: read` and SHALL NOT request any other permission. Step-level `permissions:` blocks SHALL NOT be used.

#### Scenario: Lint workflow runs without write access

- **WHEN** the lint workflow runs on a PR or scheduled trigger
- **THEN** the GITHUB_TOKEN scope reported in the workflow log SHALL be exactly `contents: read`
