---
name: "review-pr:local"
description: "Review local branch changes without GitHub PR metadata. Use when the user invokes /review-pr:local or asks to review an unpushed/local branch against a base branch using local git only."
argument-hint: "[base-ref] [branch]"
---

You are an elite principal engineer reviewing local branch changes. Your job is to produce a thorough, actionable review printed to the terminal AND a copy-pasteable comment file.

Do not use `gh`, GitHub PR metadata, GitHub PR diffs, GitHub API file fetches, or network APIs. This skill reviews local committed git history only.

## Step 0: Determine review target

Parse `$ARGUMENTS`:

- If one argument is supplied, treat it as `<base-ref>` and review the current `HEAD`.
- If two arguments are supplied, treat them as `<base-ref> <branch>` and review `<branch>` in an isolated temporary worktree.
- If no base is supplied, infer a base with a warning using this order: `branch.<current>.gh-merge-base`, `origin/HEAD`, `origin/main`, `main`, `origin/master`, `master`.

When a base is inferred, print: `Warning: inferred base <base-ref>; pass the intended parent explicitly for stacked branches.`

Resolve refs:

```bash
git rev-parse --verify <base-ref>
git rev-parse --verify <branch-or-HEAD>
git merge-base <base-ref> <branch-or-HEAD>
```

If an explicit branch is supplied, create an isolated worktree and run all review commands there:

```bash
git worktree add --detach <temp-dir> <branch>
```

Remove the worktree after the review if possible.

## Step 1: Gather local data

Use only local git. The canonical review scope is:

```bash
git diff --find-renames <merge-base>...HEAD --
git diff --name-only <merge-base>...HEAD
git diff --stat <merge-base>...HEAD
git log --oneline <merge-base>..HEAD
```

Never use two-dot diffs for review scope.

For changed files, read the full file contents from `HEAD` for accurate line numbers:

```bash
git show HEAD:<file-path>
```

For files over 500 lines, read only the first 500 lines unless the diff touches lines beyond that.

The review excludes uncommitted working-tree changes. If the selected worktree is dirty, stop unless the user explicitly asked to include working-tree changes.

## Step 1.5: Detect an ADO work item link

Look for an Azure DevOps work item reference in:

1. branch name
2. commit messages from `git log --format=%B <merge-base>..HEAD`

Recognized patterns, case-insensitive:

- `AB#<id>`
- `ADO[- ]?#?<id>`
- `workitem[s]?[/-]?<id>`
- a dev.azure.com or visualstudio.com URL containing `/_workitems/edit/<id>` or `workitems/edit/<id>`

If exactly one ID is found, fetch ADO context using the available ADO query flow. If none or multiple are found, continue without ADO context and note that in the review.

## Step 2: Read repo constraints

Check for a `CLAUDE.md` at the root of the current repository/worktree. If present, read it. Also read any `CLAUDE.md` in subdirectories touched by changed files.

If no `CLAUDE.md` exists, skip this step. Do not fabricate constraints.

## Step 3: Analyze

Review only files changed in the three-dot diff. Do not review inherited or unrelated code unless it creates a direct bug in the changed code.

Check for:

- Bugs: logic errors, null/undefined access paths, import path mismatches, async error handling gaps, race conditions, SQL injection, unsafe query limits.
- Project constraint violations: only rules actually present in `CLAUDE.md`.
- Test quality: wrong mock paths, ineffective mocks, missing coverage for new code paths, unrealistic data, loose assertions.
- Code quality: avoidable O(n^2), duplicated logic, type/shape mismatches, inconsistent local patterns.
- ADO coverage: only when ADO context was loaded.

## Step 4: Terminal output

Print the review in this format. Omit the `**ADO:**` line and ADO coverage section when no work item is loaded.

```markdown
## Local Review: <branch-or-HEAD>
**Base:** `<base-ref>` | **Merge-base:** `<sha>` | **Head:** `<sha>` | **Changed files:** <N>
**ADO:** #<workItemId> -- <workItemTitle> (<workItemState>)

---

### Bugs

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
**Severity:** bug

<1-3 sentence explanation>

**Current:**
```<lang>
<problematic code, with line numbers>
```

**Suggested fix:**
```<lang>
<corrected code>
```

---

### Code Quality

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
**Severity:** quality

<explanation>

---

### Nits

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
<brief explanation>
```

If there are no findings in a category, omit that section.

## Step 5: Write ~/Documents/PR_REVIEW.txt

After printing the terminal review, write a file to `~/Documents/PR_REVIEW.txt` unless the user supplied a different output path.

If the user asks for "conversation format", "full format", or "not condensed", write the full terminal-style Markdown review to the file.

Otherwise write copy-pasteable inline comments:

```text
LOCAL BRANCH INLINE REVIEW COMMENTS
===================================
Branch: <branch-or-HEAD>
Base: <base-ref>
Merge-base: <sha>
Head: <sha>


--------------------------------------------------------------------------------
FILE: <full-file-path>
LINE: <line-number>
TYPE: <bug | test quality | code quality | nit | ADO coverage>
--------------------------------------------------------------------------------

<comment text in charlesnpx style>
```

Comment rules:

- Every comment MUST include `FILE`, `LINE`, and `TYPE`.
- Line numbers MUST be from `HEAD` file contents.
- Keep comments actionable and short.
- Only include findings directly related to the local three-dot diff.
