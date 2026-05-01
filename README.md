# mise-en-place

A skill manager for Claude Code and Codex CLI. Installs, upgrades, and keeps in
sync the dual-target skills authored across both hosts.

## Install

For macOS or Ubuntu/Linux users who do **not** have Go installed, use the
prebuilt binary installer:

```sh
curl -fsSL https://raw.githubusercontent.com/charlesnpx/mise-en-place/main/install.sh | bash
```

The installer downloads the latest GitHub Release for your OS/architecture and
installs `mise-en-place` to `~/.local/bin`. If that directory is not on your
`PATH`, the installer prints the shell snippet to add.

To install a specific version:

```sh
curl -fsSL https://raw.githubusercontent.com/charlesnpx/mise-en-place/main/install.sh \
  | MISE_EN_PLACE_VERSION=v0.1.0 bash
```

For developers who already have Go installed:

```sh
go install github.com/charlesnpx/mise-en-place/cmd/mise-en-place@latest
```

If using `go install`, make sure `$(go env GOPATH)/bin` is on your `PATH`.

## Quick start

```sh
mise-en-place install                             # install available skills; skip optional private delegated skills
mise-en-place install --all                       # explicit alias for installing everything
mise-en-place install humanizer                   # install one skill (both targets by default)
mise-en-place install humanizer --target claude   # install only the Claude payload
mise-en-place list
mise-en-place install --all --strict              # fail if any delegated skill cannot be installed
mise-en-place upgrade --all
mise-en-place doctor
```

Release binaries are self-contained. On first use, `mise-en-place` writes its
bundled default registry and managed skill payloads to `~/.mise-en-place/`, so
commands work from any directory. Developers can set `MISE_EN_PLACE_HOME` to
point at another registry/skills tree while testing local changes.

## Concepts

- **Managed skills** live in this repo under `skills/<name>/` with a
  `skill.yaml` manifest and per-target payloads. They are released via
  per-skill semver git tags (`<skill>/<version>`).
- **Delegated skills** live in their own repos (e.g. `keyframe`, `browse`) and
  are pinned in `registry.yaml`. They may be marked `visibility: private` and
  `optional: true`. Direct install of a private delegated skill should fail
  clearly if the user lacks access; `install --all` skips optional delegated
  skills by default and `install --all --strict` turns those skips into errors.
- **Dual targets:** every managed skill can declare a `claude` payload and a
  `codex` payload. `mise-en-place port <skill> --from <host> --to <host>`
  drafts a translation between the two using the agent CLI; the human
  reviews before committing. `mise-en-place doctor` flags drift between the
  two payloads using payload tree hashes recorded in `.alignment`.

## Private delegated skills

Some registry entries are team-only. Public users can still use public managed
skills; private delegated skills are intentionally optional. The intended UX is:

- `mise-en-place install browse` — fail clearly if the repo is private and the
  user lacks access.
- `mise-en-place install --all` — skip optional private delegated skills with a
  warning.
- `mise-en-place install --all --strict` — fail if any delegated skill cannot be
  installed.

Delegated repos are cloned into `~/.cache/mise-en-place/repos/<skill>/`,
planned through their installer contract, collision-checked, and then installed.
Optional delegated failures are warnings for `install --all` unless `--strict`
is supplied.

## Existing files

When a destination file already exists, `mise-en-place` compares it with the
planned skill file:

- Identical files are treated as installed, recorded in state, and left in
  place.
- Divergent files prompt in an interactive terminal: `overwrite`, `backup`, or
  `skip`.
- `backup` renames the existing file to `<filename>.backup` (or
  `<filename>.backup.1`, `.backup.2`, and so on) before writing the skill file.
- In noninteractive shells, divergent files fail with a hint unless `--backup`
  is supplied. `--backup` performs the backup-and-overwrite flow without
  prompting.
- Skipped files are left untouched and are not recorded in state; the rest of
  the install continues where possible.

## Delegated repo compatibility contract

Delegated repos are external skill/tool repos that `mise-en-place` orchestrates
instead of owning directly. A delegated repo is compatible when it exposes a
stable installer command that supports this contract:

```sh
./install-skill.sh --plan --target all --json
./install-skill.sh --install --target all --json
./install-skill.sh --install --target all --json --install-root /tmp/stage
./install-skill.sh --uninstall --target all --json
```

Repos whose primary installer is named differently, such as `convo-porter`, may
keep that command as a wrapper, but it should expose the same flags. For example:

```sh
./install.sh --plan --target all --json
convo-porter install --plan --target all --json
```

Required flags:

- `--target claude|codex|all` — scope the operation. Default should be `all`.
- `--plan` — print the files that would be written, without modifying disk.
- `--install` — install files. This is the default when no operation flag is
  supplied.
- `--uninstall` — remove the files owned by the delegated repo.
- `--json` — write machine-readable JSON to stdout. Human logs must go to stderr.
- `--install-root <absolute-dir>` — stage an install under this directory as if
  it were `$HOME`. `mise-en-place` uses this during delegated installs so it can
  compare, prompt, back up, write, and record ownership itself.

The JSON shape is intentionally small:

```json
{
  "schema": 1,
  "name": "keyframe",
  "version": "0.26",
  "operation": "install",
  "kind": "delegated",
  "targets": {
    "claude": {
      "files": [
        {
          "path": "/Users/alice/.claude/skills/keyframe/SKILL.md",
          "sha256": "..."
        }
      ]
    },
    "codex": {
      "files": [
        {
          "path": "/Users/alice/.codex/skills/keyframe/SKILL.md",
          "sha256": "..."
        }
      ]
    }
  },
  "warnings": []
}
```

Rules:

- `schema`, `name`, `version`, `operation`, `kind`, and `targets` are required.
- `operation` is one of `plan`, `install`, or `uninstall`.
- `kind` must be `delegated`.
- File paths must be absolute.
- With `--install-root`, installed file paths in JSON must be absolute paths
  inside that root, such as `/tmp/stage/.codex/skills/keyframe/SKILL.md`.
- `sha256` is required after `install`; it is optional for `plan` and
  `uninstall`.
- stdout must contain only JSON when `--json` is set.
- The installer may use any internal language/framework; only the flags and JSON
  output are part of the contract.

`mise-en-place` uses `--plan --json` for contract validation, then runs
`--install --json --install-root <tempdir>` to stage the real planned content.
It maps staged paths back to the user's home directory and applies the same
ownership, diff, backup, and state flow used for managed skills.

## Releases

CLI releases are tagged as normal semver tags:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Pushing a `v*.*.*` tag runs GitHub Actions + GoReleaser and publishes prebuilt
archives for:

- macOS arm64
- macOS x86_64
- Linux arm64
- Linux x86_64

Skill version tags should use a distinct namespace, for example
`skill/humanizer/v2.3.0`, so they do not trigger CLI releases.

## State

- `~/.local/state/mise-en-place/state.json` — current install state
- `~/.local/state/mise-en-place/history.jsonl` — append-only audit log
- `~/.local/state/mise-en-place/state.lock` — advisory flock

## License

MIT
