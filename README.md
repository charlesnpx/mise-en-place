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
mise-en-place install                             # install default skills and default external tools
mise-en-place install --all                       # install default and experimental skills
mise-en-place install humanizer                   # install one skill (both targets by default)
mise-en-place install markitdown                  # verify/install one external tool
mise-en-place install humanizer --target claude   # install only the Claude payload
mise-en-place list
mise-en-place install --all --strict              # fail if any optional dependency cannot be installed
mise-en-place upgrade --all
mise-en-place setup azure-static-site --capability deploy
mise-en-place health
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
- **Skill renames** are declared in `registry.yaml` under `renames:` so old
  install/upgrade names resolve to the current managed or delegated skill.
- **Delegated skills** live in their own repos (e.g. `keyframe`, `browse`) and
  are declared in `registry.yaml` with either an exact `ref` pin or
  `channel: latest-release`. The latest-release channel resolves the highest
  stable `vMAJOR.MINOR.PATCH` git tag at install/upgrade time. Delegated entries
  may declare their visible skill or command names under `provides` and may be
  marked `visibility: private` and `optional: true`. Direct install of a private
  delegated skill should fail clearly if the user lacks access. Broad install
  commands skip uninstalled private optional entries unless `--strict` is
  supplied; `doctor` skips them unless one of their provided payloads is visible
  on disk.
- **Experimental skills** are managed or delegated skills listed under
  `experimental:` in `registry.yaml`. Bare `mise-en-place install` skips them.
  `mise-en-place install --all` includes them and prints a warning before each
  experimental install attempt. `mise-en-place install <skill>` also installs a
  named experimental skill with the same warning.
- **External tools** are third-party executables used by skills. They are
  declared in `registry.yaml`, checked by `doctor`, shown by `list`, and
  installed during `install --all` when `install_by_default: true`. The first
  supported manager is `pipx`; there is intentionally no automatic `pip`
  fallback.
- **Setup requirements** are declarative prerequisites exposed by managed
  manifests and delegated installer JSON. `mise-en-place setup` evaluates them
  and prints remediation without mutating shell profiles or secrets. `health`
  checks installed skills only, and `doctor` warns when installed or visible
  delegated skills are not set up.
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
- `mise-en-place install` — skip uninstalled private optional delegated skills.
- `mise-en-place install --all` — attempt experimental skills too; optional
  private delegated skills are still skipped unless they are explicit or strict.
- `mise-en-place install --all --strict` — fail if any delegated skill cannot be
  installed.

Delegated repos are cloned into `~/.cache/mise-en-place/repos/<skill>/`,
planned through their installer contract, collision-checked, and then installed.
Uninstalled private optional delegated repos are skipped before clone/fetch in
broad commands. Other optional delegated failures are skipped during broad
install and reported as warnings by `doctor` unless install `--strict` is
supplied.

Delegated source examples:

```yaml
delegated:
  ado-query:
    repo: github.com/charlesnpx/ado-query
    channel: latest-release
  agent-context:
    repo: github.com/charlesnpx/agent-context
    ref: v0.4.2
  browse:
    repo: github.com/charlesnpx/browse
    channel: latest-release
    fallback_ref: main
    visibility: private
    optional: true
  convo-relay:
    repo: github.com/charlesnpx/convo-relay
    channel: latest-release
    provides:
      - relay
      - relay:steer
experimental:
  - browse
```

Use exact `ref` pins when reproducibility matters. Use
`channel: latest-release` for delegated tools that should advance when their
own repo publishes a newer stable release. `fallback_ref` keeps untagged repos
usable until they start publishing release tags.

`provides` maps user-visible payload names back to their delegated registry
owner. When omitted, it defaults to the registry key. A provided name can belong
to only one delegated entry and cannot collide with another registry entry or a
rename.

## Visible skill dependency recovery

Installer state and visible files can drift apart after state loss, a partial
restore, or a manual skill copy. `mise-en-place doctor` inventories these
user-visible entry points:

- `~/.codex/skills/<name>/SKILL.md`
- `~/.claude/skills/<name>/SKILL.md`
- `~/.claude/commands/<name>.md`

Hidden entries, including Codex's `.system` directory, are ignored. Unknown
names that are not provided by the registry are also ignored.

A delegated entry is applicable when it is recorded in state or one of its
provided names is visible. This lets a visible private/optional payload bypass
the normal broad-command skip. Doctor resolves that owner's delegated plan once
and reuses it to check every planned file for each visible host, every planned
`tools` file, registry-declared executables on `PATH`, and declarative setup
requirements such as environment variables and authentication.

When a visible payload has no state record, doctor reports the absent state and
each missing dependency separately, with `mise-en-place install <owner>` as the
repair command. These orphan findings are warnings so unrelated healthy work is
not blocked. Integrity drift for files already owned in state remains an error.
Private optional entries with neither state nor a visible payload remain skipped
before any repository access.

## External tools

External tools are declared beside managed and delegated skills:

```yaml
external_tools:
  markitdown:
    executable: markitdown
    manager: pipx
    package: "markitdown[all]"
    install_by_default: true
    optional: true
    required_by:
      - ado-query
```

For `install --all` with `--target all` or `--target tools`, mise-en-place
checks for each default external tool on `PATH`. If it is missing, it runs:

```sh
pipx install markitdown[all]
```

If `pipx` is missing, install it with:

```sh
brew install pipx
pipx ensurepath
```

External tools are recorded in state after verification, but mise-en-place does
not own or hash their files. `uninstall <tool>` forgets the state record and
leaves the executable installed.

Python package delegated repos may also declare pipx-managed tools that belong
to that repo:

```yaml
delegated:
  keyframe:
    repo: github.com/charlesnpx/keyframe
    channel: latest-release
    tools:
      - executable: keyframe
        manager: pipx
        package: keyframe
        install_from: checkout
```

Delegated pipx tools are installed from the resolved checkout:

```sh
pipx install --force ~/.cache/mise-en-place/repos/keyframe
```

These tools are recorded under the delegated skill in state. Unlike global
external tools, `uninstall <skill>` removes delegated pipx tools with
`pipx uninstall <package>`.

Go and other self-contained delegated CLIs should usually own their `tools`
target from the delegated installer instead of using `registry.yaml tools:`.
For example, `ado-query` and `convo-relay` stage files such as
`~/.local/bin/<tool>` under `--install-root` and report them in the installer
JSON `targets.tools.files` list. Those files are owned, hashed, upgraded, and
removed through the normal delegated file flow.

## Setup and health

Skills may declare setup requirements for one or more capabilities:

- `read`
- `query`
- `write`
- `deploy`

Use setup before a skill that needs account-specific configuration:

```sh
mise-en-place setup figma-fetch --capability read
mise-en-place setup ado-query --capability query
mise-en-place setup azure-static-site --capability deploy
```

Default setup mode evaluates only: no prompts, no writes, and no shell profile
edits. `--json` returns machine-readable status. `--check` prints terse status.
`--interactive` is reserved for future approved non-secret project config writes
and requires a terminal.

Exit codes are stable:

- `0` — all requirements satisfied
- `1` — valid setup contract, but at least one requirement is missing,
  unavailable, unauthenticated, inaccessible, expired, or invalid
- `2` — setup planning/contract error
- `3` — misuse, such as an unknown skill or invalid capability filter

`mise-en-place health` runs the same checks for installed skills only. `doctor`
keeps its integrity checks and adds setup warnings for installed skills and
visible delegated orphans.

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

- `--target claude|codex|tools|all` — scope the operation. Default should be `all`.
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
  "capabilities": ["query"],
  "setup": [
    {
      "kind": "env",
      "env": "ADO_ORG",
      "value_class": "plain",
      "required_for": ["query"],
      "remediation": "Export ADO_ORG to the Azure DevOps organization name."
    }
  ],
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
    },
    "tools": {
      "files": [
        {
          "path": "/Users/alice/.local/bin/keyframe",
          "sha256": "..."
        }
      ]
    }
  },
  "warnings": [],
  "notices": []
}
```

Rules:

- `schema`, `name`, `version`, `operation`, `kind`, and `targets` are required.
- `capabilities` and `setup` are optional but recommended for skills that need
  local auth, environment variables, or external CLIs.
- Supported setup kinds are `env`, `executable`, `azure-cli-auth`,
  `azure-devops-token`, `azure-resource-group`, and `github-cli-auth`.
- Supported capability values are `read`, `query`, `write`, and `deploy`.
- `value_class` may be `plain` or `secret`; secret values are never persisted
  by setup.
- `operation` is one of `plan`, `install`, or `uninstall`.
- `kind` must be `delegated`.
- `warnings` are printed to stderr after a successful delegated install.
- `notices` are printed to stdout after a successful delegated install or an
  already-current delegated upgrade; use them for manual follow-up instructions.
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

Python package CLIs should not create `~/.local/bin` shims from their delegated
installer. Declare the pipx tool in `registry.yaml` instead. Such installers
should still accept `--target tools --json` and may return an empty tools target:
`"tools": {"files": []}`. Go and other self-contained CLIs may instead return
real `tools` target files from the delegated installer, such as
`~/.local/bin/ado-query` or `~/.local/bin/convo-relay`.

For delegated upgrades, `mise-en-place` resolves the current source first and
skips the reinstall when the installed version, resolved ref, and resolved
commit are already current, all declared pipx tools are present on `PATH`, and
all recorded installer-owned target files still exist with their recorded
hashes. Use `mise-en-place upgrade <skill> --force` to reinstall the current
resolved version.

Delegated repos using `channel: latest-release` advance by publishing stable
`vMAJOR.MINOR.PATCH` git tags. A repo that changes its packaging model should
publish a new semver line that clearly supersedes older package metadata; for
example, the Go `convo-relay` line starts at `v1.0.0` after the Python `0.4.0`
line.

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
