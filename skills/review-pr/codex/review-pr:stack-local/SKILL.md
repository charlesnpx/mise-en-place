---
name: "review-pr:stack-local"
description: "Review a local stacked branch series without GitHub PR metadata. Use when the user invokes $review-pr:stack-local or asks to review multiple local stack branches concurrently against a base branch, especially unpushed branches."
argument-hint: "<base-ref> <- <branch-prefix> ... <branch-prefix>"
---

You are reviewing a local stacked branch series. Launch one independent review subagent per stack branch, where each branch is reviewed only against the change introduced by that branch over its immediate parent.

Do not use `gh`, GitHub PR metadata, GitHub PR diffs, GitHub API file fetches, or network APIs. This skill is for unpushed or not-yet-PR branches.

## Parse the stack

Expected form:

```text
$review-pr:stack-local echo-master <- 36451.1 ... 36451.5
```

Interpret this as:

```text
echo-master <- 36451.1 <- 36451.2 <- 36451.3 <- 36451.4 <- 36451.5
```

Each branch is reviewed against its immediate parent:

- `36451.1*` against `echo-master`
- `36451.2*` against `36451.1*`
- `36451.3*` against `36451.2*`
- and so on

Support two stack syntaxes:

- Explicit chain: `<base> <- <branch-a> <- <branch-b> <- <branch-c>`
- Numeric range shorthand: `<base> <- 36451.1 ... 36451.5`

For numeric range shorthand, expand only when both endpoints share the same prefix before the final numeric segment.

## Resolve branch tokens

Branch tokens may be prefixes. Resolve each branch token using local refs only:

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

## Preflight

Before launching subagents:

```bash
git status --short
git rev-parse --verify <base-ref>
git rev-parse --verify <branch>
git merge-base <parent> <branch>
git diff --name-only <merge-base>...<branch>
git diff --stat <merge-base>...<branch>
```

The review scope is committed branch history only. Uncommitted working-tree changes are not included. If the current worktree is dirty and the dirty files appear relevant to the requested stack, stop and ask whether to continue with committed changes only.

## Launch subagents

Each branch review must run in its own subagent and its own temporary worktree.

Codex concurrency limit: keep at most 6 review subagents active at once. Start the next queued branch as soon as one subagent completes until the stack is complete.

Create a temporary worktree for each worker:

```bash
git worktree add --detach <temp-dir> <branch>
```

Each worker reviews only this branch's incremental diff:

```bash
git merge-base <parent> HEAD
git diff --find-renames <merge-base>...HEAD --
git diff --name-only <merge-base>...HEAD
git log --oneline <merge-base>..HEAD
```

Never use two-dot diffs for review scope.

## Worker prompt template

For each branch, send the worker a prompt like:

```text
Review local stack branch <branch> against parent <parent>.

Use the temporary worktree at <temp-dir>. Do not use GitHub, gh, PR metadata, or network APIs.

Canonical review scope:
- merge-base: `git merge-base <parent> HEAD`
- diff: `git diff --find-renames <merge-base>...HEAD --`
- changed files: `git diff --name-only <merge-base>...HEAD`
- commits: `git log --oneline <merge-base>..HEAD`

Review only files changed in that three-dot diff. Use HEAD file contents for line numbers.

Write the full conversation-style review, not the condensed inline-comment format, to:

~/Documents/PR_REVIEW_<safe-branch-name>.md

Include branch name, parent branch, merge-base SHA, HEAD SHA, changed-file count, diff stat, findings grouped as Bugs, Code Quality, Nits, and ADO coverage only if a work item is detected locally.

Do not modify repo files. Do not post comments. Do not write any other files.
```

## ADO detection

For each branch, detect ADO IDs from the branch name and:

```bash
git log --format=%B <merge-base>..HEAD
```

Recognized patterns:

- `AB#<id>`
- `ADO[- ]?#?<id>`
- `workitem[s]?[/-]?<id>`
- Azure DevOps work item URLs

If exactly one ID is found, fetch ADO context using the available ADO query flow. If none or multiple are found, continue without ADO context and note that in the review.

## Output

Default output path per branch:

```text
~/Documents/PR_REVIEW_<safe-branch-name>.md
```

Sanitize branch names for filenames by replacing `/`, `:`, and whitespace with `_`.

Use full conversation-style Markdown in each file, not the condensed inline-comment format.

After all workers finish, print a stack summary:

```markdown
## Stack Review Summary

Base: `<base-ref>`

| Branch | Parent | Output | Bugs | Code Quality | Nits | Status |
|---|---|---|---:|---:|---:|---|
| `<branch>` | `<parent>` | `<path>` | N | N | N | complete |
```

If a worker fails, include the failure and continue reporting completed workers.

## Cleanup

After each worker completes, remove its temporary worktree:

```bash
git worktree remove <temp-dir>
```

If removal fails because the worker is still using the directory, report the path and leave it in place rather than forcing deletion.
