---
name: "pr:description:stack"
description: "Write PR descriptions for a local stacked branch series using local three-dot diffs, shared planning context, and bounded concurrent subagents. Use when the user invokes /pr:description:stack."
argument-hint: "<base-ref> <- <branch> ... <branch> [extra context]"
---

You are writing PR descriptions for a local stacked branch series. First build shared context for the whole stack, then launch one worker per branch to write branch-specific description files under `~/Documents/pr-skills/descriptions/`, creating that directory if needed.

Use local git. Do not use GitHub PR metadata unless the user explicitly asks. It is okay to use ADO lookup when work item IDs are detected.

## Parse the stack

Expected form:

```text
/pr:description:stack echo-master <- 36451.1-location-ownership-audit-and-mapper ... 36451.5-location-sync-warning-ux, additional context
```

Interpret this as:

```text
echo-master <- 36451.1-* <- 36451.2-* <- 36451.3-* <- 36451.4-* <- 36451.5-*
```

Each branch description is scoped to the branch's own diff against its immediate parent.

Support:

- explicit chain: `<base> <- <branch-a> <- <branch-b> <- <branch-c>`
- numeric shorthand: `<base> <- 36451.1 ... 36451.5`

For numeric shorthand, expand only when both endpoints share the same prefix before the final numeric stack segment. Treat text after a comma as additional context to include in the shared planning packet.

## Resolve branches

Resolve branch tokens using local refs only:

```bash
git for-each-ref --format='%(refname:short)' refs/heads
```

Rules:

- exact branch name wins
- otherwise use prefix matching
- unless the token itself contains `backup`, ignore backup branches during prefix matching
- backup branches are refs matching `*-backup-*`, `backup/*`, or `*-bak-*`
- prefix match must resolve to exactly one non-backup local branch
- if a token is ambiguous or missing, stop and report candidates
- do not fetch remotes unless the user explicitly asks

Resolve the base ref with `git rev-parse --verify <base-ref>`.

## Shared planning step

Before launching workers, gather shared context once:

```bash
git diff --stat <base-ref>...<last-branch>
git diff --name-only <base-ref>...<last-branch>
git log --oneline <base-ref>..<last-branch>
```

Also collect:

- PR template, using the first matching path case-insensitively: `pull_request_template.md`, `.github/pull_request_template.md`, `docs/pull_request_template.md`
- ADO work item context detected from branch names and commit messages
- user-provided extra context
- parent/child branch relationships and output filenames
- any existing parent PR description files in `~/Documents/pr-skills/descriptions/`

Use this shared planning step to decide the through-line for the stack, the level of detail each PR should carry, and which context belongs only in a parent or child description.

## Worker scope

Each worker must use an isolated temporary worktree for its branch:

```bash
git worktree add --detach <temp-dir> <branch>
```

Each worker's canonical scope is:

```bash
git merge-base <parent> HEAD
git diff --find-renames <merge-base>...HEAD --
git diff --name-only <merge-base>...HEAD
git diff --stat <merge-base>...HEAD
git log --oneline <merge-base>..HEAD
```

Never use two-dot diffs for description scope.

Each worker writes only:

```text
~/Documents/pr-skills/descriptions/PR_DESC_<safe-branch-name>.txt
```

Sanitize branch names for filenames by replacing `/`, `:`, and whitespace with `_`.

## Launch subagents

Launch one subagent per stack branch. Keep at most 10 Claude subagents active at once. Start the next queued worker as soon as one completes until all descriptions are written.

Each worker gets a fully specified prompt:

```text
Write a PR description for local stack branch <branch> against parent <parent>.

Use temporary worktree <temp-dir>. Do not use GitHub, gh, or PR metadata. Use ADO context from the shared planning packet, and perform ADO lookup only if the packet explicitly asks the worker to do so.

Canonical scope:
- merge-base: `git merge-base <parent> HEAD`
- diff: `git diff --find-renames <merge-base>...HEAD --`
- changed files: `git diff --name-only <merge-base>...HEAD`
- stat: `git diff --stat <merge-base>...HEAD`
- commits: `git log --oneline <merge-base>..HEAD`

Use the shared planning packet below, the repo PR template, detected ADO context, and the parent description only when relevant. Scope the actual changes to this branch's three-dot diff.

Write to:
~/Documents/pr-skills/descriptions/PR_DESC_<safe-branch-name>.txt

If that file already exists, read it first for reusable context, then overwrite it.

Run /humanizer on the final description before saving if it preserves technical meaning.

Do not modify repo files. Do not write any other files. Do not post comments.
```

## Description requirements

Each PR description should include:

- what changed in that branch only
- why the branch exists in the stack
- useful parent/child context without duplicating sibling descriptions
- meaningful alternatives considered and rejected, only when helpful
- ADO acceptance criteria or story context when applicable
- test/verification notes

Avoid filler and generic process narration.

## Output summary

After all workers finish, print:

```markdown
## PR Description Stack Summary

Base: `<base-ref>`

| Branch | Parent | Output | ADO | Status |
|---|---|---|---|---|
| `<branch>` | `<parent>` | `<path>` | `<id or none>` | complete |
```

If a worker fails, include the failure and continue reporting completed workers.

Remove temporary worktrees after workers complete. If removal fails because a worker is still using the directory, report the path and do not force deletion.
