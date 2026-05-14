---
name: rebase-up
description: "Walk a chain of stacked branches and rebase each onto its parent, propagating accumulated changes upward. Creates timestamped backup branches first and validates nothing is lost after each step. Use when changes on a lower branch need to flow up through a branch stack, or when the user invokes rebase-up."
argument-hint: "[starting/base branch] branch-1 branch-2 branch-3 ... (ordered chain from root to tip)"
---

You are performing a careful, verified walk up a stacked branch chain, rebasing each branch onto its parent so that accumulated changes propagate upward naturally.

The default base branch is `main`, unless the user specifies a different base.

## What is a stacked branch chain?

A linear sequence of branches where each is based on the previous:

```text
main (or another specified base)
  -> branch-1
       -> branch-2
            -> branch-3
                 -> branch-4
```

You do not need to know which branch has changes. Walk the chain from bottom to top, rebasing each branch onto its parent. If a parent has new commits from direct changes or from being rebased earlier in this run, the child rebase picks them up automatically.

## Arguments

The user provides the branch chain in their request. There are three modes:

**No arguments:** Ask the user for the ordered list of branches from root to tip.

**Full chain provided:** Example: `branch-1 branch-2 branch-3 branch-4`. The first branch is the root of the chain; its base is `main` unless the user says otherwise. Every branch in the list gets rebased, starting with `branch-1` onto the base, then `branch-2` onto `branch-1`, and so on.

**Starting point plus chain:** The user may say something like "start from branch-2" and provide the rest. In this case, `branch-2`'s parent, or the base if it is the root, is fixed and not modified. Rebase from `branch-2` upward.

In all cases, you must know the complete ordered chain and the base branch before proceeding. If unsure, ask. Do not guess branch order from names; naming conventions vary.

## Step 0: Confirm the chain

Print the chain clearly:

```text
Branch chain (root -> tip):
  base: main
  1. branch-A          <- rebase onto main
  2. branch-B          <- rebase onto branch-A
  3. branch-C          <- rebase onto branch-B
  4. branch-D          <- rebase onto branch-C
```

Or, if the user specified a starting point:

```text
Branch chain (starting from branch-B):
  fixed base: branch-A (not modified)
  2. branch-B          <- rebase onto branch-A
  3. branch-C          <- rebase onto branch-B
  4. branch-D          <- rebase onto branch-C
```

Ask the user to confirm the chain is correct before proceeding. Do not continue until they confirm.

## Critical Safety Protocol: Backups First

Complete all backup steps before touching any branch.

### Step 1: Create backup branches

For every branch that will be rebased, create a timestamped backup:

```bash
git branch <branch-name>-backup-$(date +%Y%m%d-%H%M%S) <branch-name>
```

Print the full list of backup branches created:

```text
Backups created:
  branch-A  ->  branch-A-backup-20260324-143012
  branch-B  ->  branch-B-backup-20260324-143012
  branch-C  ->  branch-C-backup-20260324-143012
  branch-D  ->  branch-D-backup-20260324-143012
```

These are the safety net. If anything goes wrong, the user can restore from them.

### Step 2: Snapshot file state for verification

Before any rebase, capture a verification snapshot for each branch that will be rebased, recording its unique delta relative to its current parent:

```bash
git log --oneline <current-parent>..<branch> | wc -l
git diff --stat <current-parent>..<branch>
git diff <current-parent>..<branch> -- | shasum -a 256
```

Save all three values per branch. Compare against them after each rebase.

## Propagation Procedure

### Step 3: Rebase each branch in order

Process branches in chain order from bottom to top. For each:

```bash
git checkout <branch>
git rebase <its-parent-branch>
```

The parent is the branch immediately before it in the chain, or the base for the first branch. Since you process in order, each parent has already been rebased by the time you reach its child.

If the rebase reports "Current branch is up to date", record it as a no-op and move on after status reporting.

If there are conflicts, stop immediately. Print the conflicting files, show enough conflict-marker context to understand the issue, and ask the user how to resolve. Do not auto-resolve. Do not use `--skip` or `--abort` without explicit user instruction.

### Step 4: Verify after each rebase

After each successful rebase, run these checks. For no-ops, report the no-op and continue only after the required confirmation.

**Commit count:**

```bash
git log --oneline <parent>..<branch> | wc -l
```

Compare to the Step 2 snapshot. The count should be the same; the branch's own commits should be preserved, just replayed on a new base. A significant drop is a red flag.

**Diff stat:**

```bash
git diff --stat <parent>..<branch>
```

The set of files changed should match the snapshot. The branch's own work should still be present.

**Direct comparison against backup:**

```bash
git diff <branch> <branch-backup-TIMESTAMP> -- .
```

This shows what changed between the rebased branch and its pre-rebase backup. The only differences should be changes propagated from lower in the chain. If files disappear, large unexpected deletions appear, or unrelated changes show up, stop and alert the user.

**Commit ancestry check:**

```bash
# Commits on rebased branch that are not on the backup: expected replayed/new commits.
git log --oneline <branch-backup-TIMESTAMP>..<branch>

# Commits on backup that are not on the rebased branch: potential lost commits.
git log --oneline <branch>..<branch-backup-TIMESTAMP>
```

After a rebase, old SHAs are replaced with new ones, so the second list will usually be non-empty. Every commit listed there should have a corresponding commit, with the same message and equivalent diff, in the first list. If any commit appears lost with no rebased equivalent, stop and alert the user.

**Print status:**

```text
OK  branch-C: rebased onto branch-B
    Commits: 5 -> 5
    Files in delta: 12 (expected: 12)
    Backup diff: only propagated changes
```

Ask the user to confirm this branch looks correct before moving to the next one. Wait for confirmation.

### Step 5: Repeat for each branch

Move to the next branch and repeat Steps 3 and 4.

## After All Propagation

### Step 6: Final summary

```text
## Rebase-Up Complete

Branches processed: <count>

| # | Branch | Status | Commits (before -> after) | Backup |
|---|--------|--------|---------------------------|--------|
| 1 | branch-A | OK | 8 -> 8 | branch-A-backup-20260324-143012 |
| 2 | branch-B | no-op (up to date) | - | branch-B-backup-20260324-143012 |
| 3 | branch-C | OK | 5 -> 5 | branch-C-backup-20260324-143012 |
| 4 | branch-D | OK | 3 -> 3 | branch-D-backup-20260324-143012 |

Backup branches, safe to delete once the user is satisfied:
  branch-A-backup-20260324-143012
  branch-B-backup-20260324-143012
  branch-C-backup-20260324-143012
  branch-D-backup-20260324-143012

To delete all backups at once:
  git branch -D <space-separated list>
```

## Hard Rules

- Never force-push. The user decides when and whether to force-push.
- Never delete backup branches. Only the user deletes backups.
- Never skip verification. Every single rebase gets all checks.
- Never auto-resolve conflicts. Stop and involve the user.
- Never proceed to the next branch without explicit user confirmation that the current one looks correct.
- Never use `git merge`. This workflow is rebase-only to keep history clean.
- Never guess the branch order. If you do not know the chain, ask.
- If anything looks wrong at any point, stop, show what you see, and ask. The backups exist for exactly this reason.
