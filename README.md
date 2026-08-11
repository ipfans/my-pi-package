# my-pi-package

Opinionated installer for a full-featured [Pi](https://github.com/earendil-works/pi-mono) coding agent setup. Plugins are declared in the root **`catalog.yaml`** (pinned versions or `latest`).

## Install

Download a release binary from [GitHub Releases](https://github.com/ipfans/my-pi-package/releases), or:

```bash
go install github.com/ipfans/my-pi-package/cmd/my-pi-package@latest
```

## Quick start

```bash
my-pi-package              # interactive picker (TTY)
my-pi-package --yes        # install full catalog
my-pi-package status
my-pi-package doctor
```

## Commands

| Command | Description |
| --- | --- |
| `my-pi-package` / `install` | Install catalog packages |
| `status` | Show installed / missing packages |
| `update` | Refresh Compound (if present) + `pi update` |
| `remove <id>` | Remove a package |
| `doctor` | Environment health check |
| `skills install <source>` | Install agent skills from a marketplace repo |
| `skills remove` | Remove installed agent skills |

### Install options

```
--only core,ui          # categories or package ids
--except themes
-l, --local             # project .pi/settings.json
-y, --yes               # non-interactive
--catalog path.yaml     # custom catalog
```

### Skills (marketplace)

Installs Claude-style plugin skills into `~/.pi/agent/skills` (or `.pi/skills` with `-l`), following marketplace/`plugin.json` skill path rules (default `skills/`, explicit `skills` paths, root `SKILL.md`).

```bash
my-pi-package skills install ipfans/demo-plugin   # clone GitHub, TUI pick plugins
my-pi-package skills install ipfans/demo-plugin -y --all
my-pi-package skills remove                       # TUI pick skills to delete
my-pi-package skills remove -y --only ce-plan
```

## Development

Requires [Task](https://taskfile.dev) (optional) and Go 1.22+.

```bash
task build              # → bin/my-pi-package
task test
task run -- status
task check              # fmt + vet + test
```

Without Task:

```bash
go test ./...
go build -o bin/my-pi-package ./cmd/my-pi-package
```

## Release

Push a version tag; GitHub Actions runs [GoReleaser](https://goreleaser.com):

```bash
git tag v0.1.0
git push origin v0.1.0
```

Local snapshot:

```bash
task release:snapshot   # requires goreleaser
```

## Docs

Design notes live in [`docs/`](./docs/).
