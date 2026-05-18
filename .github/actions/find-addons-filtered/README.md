# find-addons-filtered

Composite action that wraps `home-assistant/actions/helpers/find-addons`, drops any discovered directory that lacks a `Dockerfile`, and optionally filters the resulting list against a caller-supplied set of changed files using a small list of glob patterns. It exists so the addon-discovery filter lives in one place rather than being duplicated as inline `jq`/`bash` blocks across `builder.yaml` and `lint.yaml`; callers pass the output of `tj-actions/changed-files` and read the filtered list back as `addons` / `changed_apps`.

## Inputs

| Name              | Required | Default                                 | Purpose |
|-------------------|----------|-----------------------------------------|---------|
| `changed_files`   | no       | _empty_                                 | Space- or newline-separated changed-file list (typically `tj-actions/changed-files`'s `all_changed_files`). When empty, `changed_apps` equals `addons`. |
| `monitored_globs` | no       | see [`action.yaml`](./action.yaml)      | Multi-line glob list, evaluated relative to each addon directory. Defaults: `config.{json,yaml,yml}`, `Dockerfile`, `rootfs/**`, `translations/**`, `**/*.go`, `go.mod`, `go.sum`. |

## Outputs

| Name           | Description |
|----------------|-------------|
| `addons`       | JSON array of every Dockerfile-bearing addon directory discovered. Suitable for `strategy.matrix`. |
| `changed_apps` | JSON array of addon directories whose contents intersect `changed_files` per `monitored_globs`. Equals `addons` when `changed_files` is empty or when `.github/workflows/(builder\|build-app).yaml` itself changed (workflow-rebuild escape hatch). |
| `changed`      | `"true"` when `changed_apps` is non-empty, otherwise `"false"`. Convenient for `if:` gating. |

## Usage

```yaml
- uses: actions/checkout@<pinned>
- id: changed
  uses: tj-actions/changed-files@<pinned>
- id: addons
  uses: ./.github/actions/find-addons-filtered
  with:
    changed_files: ${{ steps.changed.outputs.all_changed_files }}
# strategy.matrix.app: ${{ fromJSON(steps.addons.outputs.changed_apps) }}
```
