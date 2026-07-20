---
name: rebase-up
description: "Walk a chain of stacked branches and rebase each onto its parent, propagating any accumulated changes upward. Creates timestamped backup branches first and validates nothing is lost after each step. Use when changes on a lower branch need to flow up through the stack. By default, starts without approval when the base and ordered chain are known, continues after clean verification, and stops only for missing inputs, blockers, non-trivial conflicts, or suspicious rebase results."
argument-hint: "[starting-branch] branch-1 branch-2 branch-3 ... (ordered chain from root to tip)"
---

You are performing a careful, verified walk up a **stacked branch chain**, rebasing each branch onto its parent so that any changes encountered along the way propagate upward naturally.

Default to autonomous verified propagation:

- Do not ask for approval to start when the base branch and complete ordered chain are known.
- Do not ask for confirmation after each branch when the rebase and verification are clean.
- Stop and ask only for missing or ambiguous inputs, dirty-state blockers, command failures, non-obvious or non-trivial conflicts, verification mismatches, possible commit loss, or any action that needs user judgment.
- If the user explicitly asks for per-step approval, use `rebase-up:explicit-approval` instead.

The default base branch is **`main`**, unless the user specifies a different base.

## What is a stacked branch chain?

A linear sequence of branches where each is based on the previous:

```
main (or another specified base)
  └─ branch-1
       └─ branch-2
            └─ branch-3
                 └─ branch-4
```

You don't need to know *which* branch has changes. You simply walk the chain from bottom to top, rebasing each branch onto its parent. If a parent has new commits (from direct changes or from being rebased earlier in this same run), the rebase picks them up automatically.

## How arguments work

`$ARGUMENTS` provides the branch chain. There are three modes:

**No arguments:** Ask the user for the ordered list of branches from root to tip.

**Full chain provided:** e.g., `branch-1 branch-2 branch-3 branch-4`. The first branch is the root of the chain (its base is `main` unless the user says otherwise). Every subsequent branch rebases onto the one before it. ALL branches in the list get rebased — starting with branch-1 onto the base, then branch-2 onto branch-1, etc.

**Starting point + chain:** The user may say something like "start from branch-2" and provide the rest. In this case, branch-2's parent (the branch before it, or main if it's the root) is the fixed base, and you rebase from branch-2 upward.

In all cases, **you MUST know the complete ordered chain and the base branch before proceeding.** If unsure, ASK. Do not guess branch order from names — naming conventions vary.

## Step 0: Resolve the chain

Print the chain clearly:

```
Branch chain (root → tip):
  base: main
  1. branch-A          ← rebase onto main
  2. branch-B          ← rebase onto branch-A
  3. branch-C          ← rebase onto branch-B
  4. branch-D          ← rebase onto branch-C
```

Or if the user specified a starting point:

```
Branch chain (starting from branch-B):
  fixed base: branch-A (not modified)
  2. branch-B          ← rebase onto branch-A
  3. branch-C          ← rebase onto branch-B
  4. branch-D          ← rebase onto branch-C
```

If the base and ordered chain are complete and unambiguous, proceed without asking for confirmation. If the base branch, parent branch for the starting point, or branch order is unknown or ambiguous, ask only for the missing information. Do not guess branch order from names.

Before creating backups, run a preflight:

```bash
git status --short --branch
git rev-parse --verify <base-or-fixed-parent>
git rev-parse --verify <branch-to-rebase>
```

Also verify there is no rebase, merge, cherry-pick, or revert already in progress. If the worktree is dirty, an operation is already in progress, or any branch cannot be resolved, stop and ask the user how to proceed.

## CRITICAL SAFETY PROTOCOL — Backups First

**You MUST complete ALL backup steps before touching ANY branch.** No exceptions. No shortcuts.

### Step 1: Create backup branches for EVERY branch that will be rebased

For **every** branch that will be rebased, create a timestamped backup:

```bash
git branch <branch-name>-backup-$(date +%Y%m%d-%H%M%S) <branch-name>
```

Print the full list of backup branches created:

```
Backups created:
  branch-A  →  branch-A-backup-20260324-143012
  branch-B  →  branch-B-backup-20260324-143012
  branch-C  →  branch-C-backup-20260324-143012
  branch-D  →  branch-D-backup-20260324-143012
```

These are the safety net. If anything goes wrong, the user can restore from these.

### Step 2: Snapshot file state for verification

Before any rebase, capture a verification snapshot for **each branch that will be rebased**, recording its unique delta relative to its current parent:

```bash
git log --oneline <current-parent>..<branch> | wc -l          # unique commit count
git diff --stat <current-parent>..<branch>                     # files changed
git diff <current-parent>..<branch> -- | sha256sum             # content hash of the delta
```

Save all three values per branch — you will compare against them after each rebase.

Also identify the actual incremental change the branch contributes over the branch it targets. For each branch, inspect the commits and diff relative to its current parent:

```bash
git log --oneline <current-parent>..<branch>
git diff --name-status <current-parent>..<branch>
git diff <current-parent>..<branch> --
```

Write a short contribution note for each branch: what this branch is actually adding, changing, or removing, and which files carry that work. Use this note as context during the rebase to decide which changes matter, especially when resolving conflicts or interpreting backup diffs. The goal is to preserve the branch's own contribution while allowing already-rebased parent changes to propagate upward.

## Propagation Procedure

### Step 3: Rebase each branch, one at a time, walking up the chain

Process branches **in chain order** from bottom to top. For each:

```bash
git checkout <branch>
git rebase <its-parent-branch>
```

The parent is whichever branch comes immediately before it in the chain (or main/the base for the first one). Since you process in order, each parent has already been rebased by the time you reach its child.

**If the rebase reports "Current branch is up to date":** That's fine - no changes to propagate at this level. Record it as a no-op, print the status, and continue to the next branch without asking for confirmation.

**If there are conflicts:** Inspect them immediately.

- Resolve automatically only when the resolution is obvious and trivial: the intended file contents are directly implied by surrounding context, both sides are preserving the same intent, and you can explain the resolution in one sentence.
- After any automatic conflict resolution, stage the resolved files, continue the rebase, and run the full verification below.
- If the conflict is semantic, touches user intent, deletes or rewrites meaningful code, involves generated artifacts without a deterministic regeneration path, or is uncertain in any way, stop. Print the conflicting files, show enough conflict-marker context to understand the issue, and ask the user how to resolve.

Do not use `git rebase --skip` or `git rebase --abort` without explicit user instruction.

**If the rebase succeeds cleanly:** proceed to verification.

### Step 4: Verify after EACH rebase

After each successful rebase (skip this for no-ops), run all of these checks:

**4a. Commit count:**
```bash
git log --oneline <parent>..<branch> | wc -l
```
Compare to the Step 2 snapshot. The count should usually be the same (the branch's own commits are preserved, just replayed on a new base). A significant or unexplained drop is a red flag.

**4b. Diff stat:**
```bash
git diff --stat <parent>..<branch>
```
The set of files changed should match the snapshot. The branch's own work should all still be present.

**4c. Direct comparison against backup:**
```bash
git diff <branch> <branch-backup-TIMESTAMP> -- .
```
This shows what changed between the rebased branch and its pre-rebase backup. The **only** differences should be changes propagated from lower in the chain. If you see files disappearing, large unexpected deletions, or changes unrelated to the propagation — STOP and alert the user.

**4d. Commit ancestry check:**
```bash
# Commits on rebased branch that aren't on the backup (new/replayed commits — expected)
git log --oneline <branch-backup-TIMESTAMP>..<branch>

# Commits on backup that aren't on the rebased branch (potential lost commits)
git log --oneline <branch>..<branch-backup-TIMESTAMP>
```
After a rebase, old SHAs are replaced with new ones, so the second list will be non-empty — but every commit listed there should have a corresponding commit (same message/diff) in the first list. If any commit appears lost with no rebased equivalent, STOP and alert the user.

**4e. Print status and continue on clean results:**
```
OK  branch-C: rebased onto branch-B
    Commits: 5 → 5
    Files in delta: 12 (expected: 12)
    Backup diff: only propagated changes
```

If all checks are clean and any conflict resolution was obvious and trivial, continue to the next branch without asking. If any check is suspicious or cannot be confidently explained as expected propagation from lower in the chain, stop and ask.

### Step 5: Repeat for each branch up the chain

Move to the next branch. Repeat Steps 3-4.

## After All Propagation

### Step 6: Final summary

```
## Rebase-Up Complete

Branches processed: <count>

| # | Branch | Status | Commits (before → after) | Backup |
|---|--------|--------|--------------------------|--------|
| 1 | branch-A | OK | 8 → 8 | branch-A-backup-20260324-143012 |
| 2 | branch-B | no-op (up to date) | — | branch-B-backup-20260324-143012 |
| 3 | branch-C | OK | 5 → 5 | branch-C-backup-20260324-143012 |
| 4 | branch-D | OK | 3 → 3 | branch-D-backup-20260324-143012 |

Backup branches (safe to delete once you're satisfied):
  branch-A-backup-20260324-143012
  branch-B-backup-20260324-143012
  branch-C-backup-20260324-143012
  branch-D-backup-20260324-143012

To delete all backups at once:
  git branch -D <space-separated list>
```

## Hard Rules

- **NEVER force-push.** The user decides when and whether to force-push.
- **NEVER delete backup branches.** Only the user deletes backups.
- **NEVER skip verification.** Every single rebase gets all checks.
- **NEVER auto-resolve non-obvious or non-trivial conflicts.** Stop and involve the user.
- **NEVER ask for routine start or per-branch approval** when the inputs are complete and verification is clean.
- **NEVER use `git merge`.** This workflow is rebase-only to keep history clean.
- **NEVER guess the branch order.** If you don't know the chain, ask.
- **NEVER use `git rebase --skip` or `git rebase --abort`** without explicit user instruction.
- If anything looks wrong at any point — unexpected diff, missing commits, weird conflict — **STOP**, show what you see, and ask. The backups exist for exactly this reason. Err heavily on the side of caution.
