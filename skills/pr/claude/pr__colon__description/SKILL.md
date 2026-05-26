---
name: "pr:description"
description: "Write a PR description for a local branch using only the branch's own diff against its inferred or explicit base. Use when the user invokes /pr:description or asks to draft a PR description for a branch."
argument-hint: "[branch] [base-ref]"
---

You are writing a PR description for a local branch. Produce a human, useful PR description and save it to a branch-specific file under `~/Documents/pr-skills/descriptions/`, creating that directory if needed.

Use local git first. Do not use GitHub PR metadata unless the user explicitly asks and the branch is already pushed. It is okay to use ADO lookup when an ADO work item is detected.

## Parse arguments

Supported forms:

- `/pr:description <branch>`
- `/pr:description <branch> <base-ref>`
- `/pr:description <base-ref> <- <branch>`

If no branch is provided, use the current branch. If no base is provided, infer one with this order:

1. The previous branch in the same numeric stack, such as `36451.1-*` for `36451.2-*`.
2. `branch.<current>.gh-merge-base`
3. `origin/HEAD`
4. `origin/main`
5. `main`
6. `origin/master`
7. `master`

When inferring a base, print the selected base and why it was chosen. If a stacked parent is inferred, make sure exactly one non-backup local branch matches it.

## Resolve stacked parents

For branch names like `36451.2-replacement-writer-and-orchestrator`, infer the parent by replacing the final numeric stack segment with the previous number and prefix-matching local branches:

```bash
git for-each-ref --format='%(refname:short)' refs/heads
```

Rules:

- exact branch name wins
- otherwise use prefix matching
- unless the token itself contains `backup`, ignore backup branches during prefix matching
- backup branches are refs matching `*-backup-*`, `backup/*`, or `*-bak-*`
- prefix match must resolve to exactly one non-backup local branch
- if ambiguous or missing, fall back to the next base inference option and warn

## Gather context

Use this canonical local scope:

```bash
git rev-parse --verify <base-ref>
git rev-parse --verify <branch>
git merge-base <base-ref> <branch>
git diff --find-renames <merge-base>...<branch> --
git diff --name-only <merge-base>...<branch>
git diff --stat <merge-base>...<branch>
git log --oneline <merge-base>..<branch>
```

Never use a two-dot diff for PR description scope.

If reviewing a branch other than the current checkout, use an isolated temporary worktree:

```bash
git worktree add --detach <temp-dir> <branch>
```

Remove the worktree after writing the description if possible.

If the selected worktree is dirty, exclude uncommitted changes. Stop only if the user explicitly asked to include working-tree changes and the dirty state would change the description.

## PR template

Use the first matching PR template found in this order, case-insensitively:

1. `pull_request_template.md`
2. `.github/pull_request_template.md`
3. `docs/pull_request_template.md`

Prefer `.github/pull_request_template.md` when several equivalent templates exist and the repo convention is unclear. Preserve useful headings from the template, but remove placeholder instructions that do not belong in the final PR description.

## Parent description context

Write output files to:

```text
~/Documents/pr-skills/descriptions/PR_DESC_<safe-branch-name>.txt
```

Sanitize branch names for filenames by replacing `/`, `:`, and whitespace with `_`.

If the selected base is a real local stack parent, look for its description file in the same directory and read it if present. Use it only for context that helps the new PR make sense, such as where this branch fits in the stack or a decision carried forward. Do not duplicate the parent PR's detailed changes. Do not pull in a parent description for root bases like `echo-master`, `main`, `master`, `origin/main`, or `origin/master`.

If the current branch's output file already exists, read it first to determine whether any still-relevant context should be preserved. Then overwrite it with the new description.

## ADO context

Detect Azure DevOps work item IDs from:

- branch name
- commit messages from `git log --format=%B <merge-base>..<branch>`

Recognized patterns, case-insensitive:

- `AB#<id>`
- `ADO[- ]?#?<id>`
- `workitem[s]?[/-]?<id>`
- Azure DevOps work item URLs containing `/_workitems/edit/<id>` or `workitems/edit/<id>`

If exactly one ID is found, use the available ADO query flow to fetch title, description, acceptance criteria, comments, linked work, and attachments when relevant. If lookup fails, continue without ADO context and mention that the description is based on local git context.

## Write the description

The description should be useful to a future reviewer and maintainer. Include:

- what changed in this branch, scoped only to `<merge-base>...<branch>`
- our reasoning for concluding this was the correct approach
- meaningful things considered but not done, only when they help explain a design choice, boundary, tradeoff, or future maintenance concern
- test/verification notes based on commits, diff, and user-provided context
- stack context when this branch is based on another stack branch
- ADO context when available and relevant
- any other useful information that might help a future reviewer or maintainer understand, modify, debug, or extend the work

Do not pad the description with process notes. Be conservative about whether "things considered but not done" or extra background is useful; include them only when they help review or future maintenance.

Before saving, run `/humanizer` on the drafted PR description and apply the rewrite if it preserves the technical meaning.

After writing the file, print the output path and a short summary of the base, merge-base, branch, and ADO ID if one was used.
