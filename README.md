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
mise-en-place install --all                       # install every skill in registry
mise-en-place install humanizer                   # install one skill (both targets by default)
mise-en-place install humanizer --target claude   # install only the Claude payload
mise-en-place list
mise-en-place upgrade --all
mise-en-place doctor
```

## Concepts

- **Managed skills** live in this repo under `skills/<name>/` with a
  `skill.yaml` manifest and per-target payloads. They are released via
  per-skill semver git tags (`<skill>/<version>`).
- **Delegated skills** live in their own repos (e.g. `keyframe`, `browse`) and
  are pinned in `registry.yaml`. `mise-en-place install <skill>` clones them
  at the pinned tag and runs their `install-skill.sh`.
- **Dual targets:** every managed skill can declare a `claude` payload and a
  `codex` payload. `mise-en-place port <skill> --from <host> --to <host>`
  drafts a translation between the two using the agent CLI; the human
  reviews before committing. `mise-en-place doctor` flags drift between the
  two payloads using payload tree hashes recorded in `.alignment`.

## State

- `~/.local/state/mise-en-place/state.json` — current install state
- `~/.local/state/mise-en-place/history.jsonl` — append-only audit log
- `~/.local/state/mise-en-place/state.lock` — advisory flock

## License

MIT
