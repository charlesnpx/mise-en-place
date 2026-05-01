# mise-en-place

A skill manager for Claude Code and Codex CLI. Installs, upgrades, and keeps in
sync the dual-target skills authored across both hosts.

## Install

```sh
brew tap charlesnpx/tap && brew install mise-en-place
# or
go install github.com/charlesnpx/mise-en-place/cmd/mise-en-place@latest
```

## Quick start

```sh
mise-en-place install --all                       # install available managed skills; skip optional private delegated skills
mise-en-place install humanizer                   # install one skill (both targets by default)
mise-en-place install humanizer --target claude   # install only the Claude payload
mise-en-place list
mise-en-place install --all --strict              # fail if any delegated skill cannot be installed
mise-en-place upgrade --all
mise-en-place doctor
```

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

The current build has the metadata and skip/error behavior in place. Until
delegated repo cloning/running `install-skill.sh` is implemented, `install --all`
skips all delegated skills by default and `--strict` fails on them.

## State

- `~/.local/state/mise-en-place/state.json` — current install state
- `~/.local/state/mise-en-place/history.jsonl` — append-only audit log
- `~/.local/state/mise-en-place/state.lock` — advisory flock

## License

MIT
