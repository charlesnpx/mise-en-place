---
name: "review-pr:no-file"
description: "Perform a principal-engineer-level code review of a GitHub PR, cross-referenced against its linked Azure DevOps work item when one exists, without writing a review file. Use when the user invokes /review-pr:no-file or asks to review a PR without saving comments to disk."
argument-hint: "[pr-number]"
---

You are an elite principal engineer reviewing PR #$ARGUMENTS. Your job is to produce a thorough, actionable review printed to the terminal only. Do not write `~/Documents/PR_REVIEW.txt` or any other review file.

## Step 0: Determine the repository

Before gathering data, resolve which GitHub repository the PR belongs to:

1. Run `gh repo view --json nameWithOwner -q .nameWithOwner` from the current working directory. That value is `<OWNER>/<REPO>` and should be used in every `gh` call below.
2. If that command fails (not inside a git repo, or no GitHub remote), ask the user which repo to review before continuing.

Use the resolved `<OWNER>/<REPO>` wherever the instructions below reference the repository.

## Step 1: Gather data in parallel

Launch agents in parallel to collect the raw data. Agent A always runs; Agent B only runs if Step 1.5 finds an ADO work item reference.

### Agent A — GitHub PR data (subagent_type: general-purpose)

Fetch ALL of the following for PR #$ARGUMENTS and return them in a single structured response:

1. `gh pr view $ARGUMENTS --repo <OWNER>/<REPO> --json title,body,state,baseRefName,headRefName,files,commits,reviews,comments,additions,deletions,changedFiles`
2. `gh pr diff $ARGUMENTS --repo <OWNER>/<REPO>` — the full diff
3. For every file changed in the PR, fetch the **full file contents from the PR branch** so we have accurate line numbers (not just diff hunks):
   ```
   gh api "repos/<OWNER>/<REPO>/contents/<file-path>" --method GET -f ref=<headRefName> --jq '.content' | base64 -d
   ```
   For files over 500 lines, fetch just the first 500 lines unless the diff touches lines beyond that.
4. Any existing review comments from other reviewers (inline and top-level)

Return all of this structured data. Do NOT summarize or analyze — just collect and return the raw information.

### Step 1.5 — Detect an ADO work item link (before launching Agent B)

Look for an Azure DevOps work item reference in the following locations, in order, and stop at the first match:

1. PR title
2. PR body/description
3. PR head branch name
4. Commit messages on the PR

Recognized patterns (case-insensitive):
- `AB#<id>` (GitHub-to-ADO integration shorthand)
- `ADO[- ]?#?<id>`
- `workitem[s]?[/-]?<id>`
- A dev.azure.com or visualstudio.com URL containing `/_workitems/edit/<id>` or `workitems/edit/<id>`

If none of those patterns yield an ID, do NOT launch Agent B. Record "No ADO work item linked." and proceed without the ADO context — the review should still happen. Do not invent an ID or speculate about one.

### Agent B — ADO work item (subagent_type: ado-query) — only if an ID was found

Fetch the full work item for the ID extracted in Step 1.5, including:
- Title, state, description, acceptance criteria, test cases
- ALL comments (with dates and authors)
- Any attached images (downloaded and described per the ado-query agent protocol)
- Child/parent work items if any

If the ado-query agent returns an error (work item not found, project access denied, etc.), treat the same as "No ADO work item linked" — note the failure and continue without ADO context.

## Step 2: Read the repo's CLAUDE.md (if one exists)

Check for a `CLAUDE.md` at the root of the current repository. If present, read it — it contains project-specific constraints that every review comment should be checked against. Also read any `CLAUDE.md` in subdirectories that are touched by the PR's changed files.

If no `CLAUDE.md` exists, skip this step — do not fabricate constraints.

## Step 3: Analyze

With all data collected, perform your review. For each file changed in the PR, examine:

### Bug Detection
- Logic errors, off-by-one, null/undefined access paths
- Mismatched import paths (especially after file moves/renames — verify mock paths in tests match actual import sources)
- Inconsistent fallback values (e.g., `?? []` where an object is expected)
- Filter/search logic where the predicate doesn't match what generates the options
- Race conditions, missing error handling on async paths
- SQL injection, missing parameterization, driver-specific parameter limits (e.g., MSSQL 2100)

### Project Constraint Violations
Check every changed file against the project's `CLAUDE.md` constraints (if one was found in Step 2). Pay particular attention to "Do Not" / "Key Constraints" / "Conventions" sections. Examples of the kinds of rules these files commonly encode:
- Forbidden types (e.g., `any`)
- Pinned library versions or forbidden imports
- Required boundary transformations (casing conversions, schema validation)
- Required config access patterns (runtime config object vs `process.env`)
- Package manager choice
- Banned utility libraries or patterns

Only flag violations of rules that actually appear in the project's CLAUDE.md. Do not impose rules from other projects.

### Test Quality
- Mocks targeting wrong module paths (especially after refactors that move functions between files)
- Tests that pass by accident (e.g., mock has no effect but real function produces same output)
- Missing test coverage for new code paths
- Mock data that doesn't match real DB column names or types
- Assertions that are too loose to catch regressions

### Code Quality
- O(n^2) or worse algorithms where O(n) is trivial (e.g., `filter` + `indexOf` for uniqueness instead of `Set`/`Map`)
- Duplicated logic that should be shared
- Type mismatches between server response shape and client type expectations
- Inconsistent patterns for the same operation (e.g., `Map` vs plain object for the same conceptual return type)

### ADO Work Item Coverage (only if ADO context was loaded)
Compare the PR's changes against the ADO work item's:
- Acceptance criteria / test cases — which are addressed, which are not
- Functional and non-functional requirements
- Decisions made in ADO comments (these often override or clarify the original description)
- Note: a PR may intentionally cover only a subset of the ticket. Flag gaps but don't assume they're oversights — note which related PRs (if any) are also linked to the work item.

If no ADO context was loaded, skip this section entirely — do not synthesize coverage analysis from the PR description alone.

## Step 4: Terminal output

Print your review to the terminal in this exact format. Omit the `**ADO:**` line entirely when no work item is linked, and omit the ADO Coverage section at the bottom as well.

```
## PR #<number>: <title>
**<additions>+ / <deletions>-** across **<changedFiles> files** | Base: `<baseRefName>` | Head: `<headRefName>` | State: `<state>`
**ADO:** #<workItemId> — <workItemTitle> (<workItemState>)

---

### Bugs

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
**Severity:** bug

<1-3 sentence explanation of what's wrong and why>

**Current:**
```<lang>
<the problematic code, with line numbers>
```

**Suggested fix:**
```<lang>
<the corrected code>
```

---

### Code Quality

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
**Severity:** quality

<explanation>

**Current:**
```<lang>
<code>
```

**Suggested fix:**
```<lang>
<code>
```

---

### Nits

#### <N>. <Short title>
**File:** `<file-path>:<line-number>`
<brief explanation — no code blocks needed for trivial issues>

---

### ADO #<workItemId> Coverage

**Test cases addressed by this PR:**
- TC<N>: <name> -- covered / partially covered / not covered

**Gaps or concerns:**
- <bullet points noting any gaps, with links to relevant ADO comments>
  ADO comment by <author> on <date>: "<relevant quote>"

**Related PRs:** #<number> (also linked to this work item)
```

### Terminal output rules:
- Every finding with a code suggestion MUST include the exact file path and line number from the PR branch (not diff line numbers)
- Group findings by severity: Bugs first, then Code Quality, then Nits
- Number findings sequentially within each group
- If a finding was also flagged by another reviewer (Gemini, SonarQube, etc.), note that: "Also flagged by <reviewer>."
- If there are no findings in a category, omit that section entirely
- For ADO coverage (when present), reference specific test case numbers from the acceptance criteria
- Keep explanations concise — lead with what's wrong, not background context
- Do NOT suggest improvements that aren't directly related to the PR's changes (don't review untouched code)

## Step 5: Stop without writing a file

After printing the terminal review, stop. Do not create, update, or delete `~/Documents/PR_REVIEW.txt`, and do not write the review to any other file.
