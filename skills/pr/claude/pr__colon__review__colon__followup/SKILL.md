---
name: "pr:review:followup"
description: "Assess how a GitHub PR's author responded to your latest review: replies, reactions, resolved threads, and the code changes made since. Use when the user asks for a follow-up review, asks whether review feedback was addressed, or when a review is re-requested on a PR they already reviewed."
argument-hint: "[pr-number-or-url]"
---

You are a principal engineer following up on your own review of PR $ARGUMENTS. Assess every comment you posted in your latest review: how the author responded, whether the code changes fix what you raised, and whether the follow-up changes introduced new problems. Print a summary to the terminal AND write a follow-up file.

This is a read-only task. Never post comments, replies, reviews, or reactions, never resolve threads, and never push. Write only the follow-up file described in Step 7.

## Source of truth

The review as posted on GitHub is the only source of truth for what was asked. The reviewer often rewords the comments drafted by `/pr:review`, adds comments of their own, and leaves some suggestions out. Do not read or compare against `~/Documents/pr-skills/reviews/PR_REVIEW_*.txt`. A suggestion that was never posted is out of scope, even if a local file mentions it.

## Step 0: Resolve the repository, PR, and reviewer

1. If `$ARGUMENTS` is a URL like `https://github.com/<OWNER>/<REPO>/pull/<NUMBER>`, take `<OWNER>/<REPO>` and `<NUMBER>` from it. Otherwise treat `$ARGUMENTS` as the PR number and run `gh repo view --json nameWithOwner -q .nameWithOwner`. If that fails, ask the user which repo to use.
2. The reviewer is the authenticated GitHub user: `gh api user --jq .login`. Call it `<REVIEWER>`.

Use the resolved `<OWNER>/<REPO>`, `<NUMBER>`, and `<REVIEWER>` in every command below. All `gh api` REST calls are GETs; add `--method GET` whenever you pass `-f` or `-F`.

## Step 1: Gather data

1. PR metadata:
   ```
   gh pr view <NUMBER> --repo <OWNER>/<REPO> --json number,title,author,state,isDraft,baseRefName,headRefName,headRefOid,url
   ```
2. All reviews, oldest first:
   ```
   gh api repos/<OWNER>/<REPO>/pulls/<NUMBER>/reviews --paginate --jq '.[] | {id, login: .user.login, state, submitted_at, commit_id, body}'
   ```
3. All review threads with replies, reactions, and resolution state:
   ```
   gh api graphql --paginate -F owner=<OWNER> -F name=<REPO> -F number=<NUMBER> -f query='
   query($owner: String!, $name: String!, $number: Int!, $endCursor: String) {
     repository(owner: $owner, name: $name) {
       pullRequest(number: $number) {
         reviewThreads(first: 50, after: $endCursor) {
           pageInfo { hasNextPage endCursor }
           nodes {
             id isResolved isOutdated path line originalLine startLine
             resolvedBy { login }
             comments(first: 100) {
               nodes {
                 databaseId url createdAt body
                 author { login }
                 pullRequestReview { databaseId }
                 originalCommit { oid }
                 reactions(first: 50) { nodes { content user { login } } }
               }
             }
           }
         }
       }
     }
   }'
   ```
4. Top-level PR conversation comments, which is where authors sometimes answer a review body:
   ```
   gh api repos/<OWNER>/<REPO>/issues/<NUMBER>/comments --paginate --jq '.[] | {login: .user.login, created_at, body}'
   ```

## Step 2: Pick the review to follow up on

From the reviews by `<REVIEWER>`, ignore `PENDING` reviews. Some reviews contain only replies to existing threads (they have an empty body and start no thread). Ignore those too.

The target review is the most recent remaining review by `<REVIEWER>` that either has a non-empty body or is the review of the first comment of at least one thread (match `comments.nodes[0].pullRequestReview.databaseId` to the review `id`).

- If `<REVIEWER>` has no such review, print "No review by <REVIEWER> to follow up on." and stop without writing a file.
- If the target review was dismissed, still assess it and say it was dismissed.

Record the target review's `id`, `state`, `submitted_at`, and `commit_id`. Also note threads that `<REVIEWER>` started in earlier reviews and that are still unresolved; they get a short section in the output.

## Step 3: Gather the changes made since the review

1. Compare the commit the review was made on with the current head:
   ```
   gh api repos/<OWNER>/<REPO>/compare/<review commit_id>...<headRefOid> --jq '{status, ahead_by, behind_by, commits: [.commits[] | {sha: .sha[0:8], message: .commit.message, date: .commit.committer.date}], files: [.files[] | {filename, status, patch}]}'
   ```
   If `status` is `ahead`, the compare shows exactly the changes made since the review.
2. If `status` is `diverged` or the compare fails, the branch was rebased or force-pushed after the review, and the compare also contains changes from the base branch. Say so in the output, and do not treat the compare's file list as the author's changes. Instead:
   - Limit the changed files to the PR's current files (`gh pr view <NUMBER> --repo <OWNER>/<REPO> --json files`) plus the files the target review commented on.
   - For each of those files, fetch it at the review's `commit_id` and at `headRefOid` with the contents command below, and compare the two versions yourself.
   - A difference in one of those files can still come from the base branch. If a change has nothing to do with the PR's purpose or the comments, leave it out.
   - List the commits from `gh pr view <NUMBER> --repo <OWNER>/<REPO> --json commits` whose committed date is after the review.
3. For every file that a target-review comment points at or that changed since the review, fetch the current contents from the head commit so line numbers are accurate (use `ref=<review commit_id>` for the version the review saw):
   ```
   gh api "repos/<OWNER>/<REPO>/contents/<file-path>" --method GET -f ref=<headRefOid> --jq '.content' | base64 -d
   ```

If the head commit is the same as the review's `commit_id`, no code has changed since the review. Assess the replies only.

## Step 4: Read the repo's CLAUDE.md (if one exists)

Read `CLAUDE.md` at the repository root and in directories touched by the changed files. Use its constraints when judging fixes and new code. If none exists, skip this step.

## Step 5: Assess each comment in the target review

For each thread whose first comment belongs to the target review, and for the review body if it raises anything actionable, collect:

- **Comment:** the comment as posted, with its file and line.
- **Responses:** replies from anyone other than `<REVIEWER>`, reactions from anyone other than `<REVIEWER>`, whether the thread is resolved and by whom, and whether it is outdated.
- **Code:** the current code at the comment's location, and any change since the review that relates to the comment, even if it moved to another file.

Then give the comment exactly one status:

| Status | Meaning |
|---|---|
| `addressed` | The code now fixes what the comment raised, or the change asked for was made. |
| `partly addressed` | Some of it was fixed, or the fix covers only some of the cases. Say what remains. |
| `not addressed` | Nothing relevant changed and no reply explains why, even if the thread was resolved or got a reaction. |
| `pushed back` | The author disagreed or chose not to change it. Say whether their reasoning holds up and why. |
| `answered` | The comment was a question and the author answered it. Say whether the answer is correct and whether it calls for a code change. |
| `deferred` | The author said it will be handled later, such as in another PR or work item. Note where, if they said. |
| `closed by you` | `<REVIEWER>` resolved the thread or replied to close it. Still check the code and flag it if the fix is missing. |

Rules for judging:

- Judge by the code, not by the response. A resolved thread or a 👍 without a matching code change is `not addressed`. A fix with no reply is still `addressed`.
- An outdated thread only means the lines changed. Read the new code before deciding.
- 👀 means the author saw the comment, not that it was fixed. 👎 or 😕 suggests disagreement. Look for a reply that explains it.
- When the author pushed back or answered, check their claim against the code. Agree when they are right.
- Treat any comment from someone other than the PR author or `<REVIEWER>` as context only.

## Step 6: Check the follow-up changes

Review only the code changed since the target review, using the same standards as `/pr:review`: bugs, project constraint violations, test quality, and code quality. Report only problems that were introduced or left behind by the follow-up changes. Do not re-review code that did not change, and do not repeat comments from the target review.

## Step 7: Output

### Terminal

```
## PR #<number> follow-up: <title>
Review by <REVIEWER> on <submitted_at> (<state>) | <K> commits since | Head: `<short headRefOid>`

**<A> addressed · <P> partly · <N> not addressed · <B> pushed back · <Q> answered · <D> deferred · <C> closed by you**

### Needs your attention
- `<file>:<line>` <status>: <one line on what is left or what to decide>

### Addressed
- `<file>:<line>` <one line on the fix>

### New issues in follow-up changes
- `<file>:<line>` <severity>: <one line>
```

Omit any section that has no entries. Put `partly addressed`, `not addressed`, `pushed back` you disagree with, and new issues under "Needs your attention".

### File

Write `~/Documents/pr-skills/reviews/PR_REVIEW_<repo-name>_<pr-number>_<author>_addressed_<n>.txt`, creating the directory if needed.

- `<repo-name>` is the `<REPO>` part of `<OWNER>/<REPO>`, with any character outside `[A-Za-z0-9._-]` replaced by `_`.
- `<author>` is the PR author's GitHub login (`author.login` from Step 1), with the same character replacement.
- `<n>` is the smallest positive integer for which the file does not exist yet, so earlier follow-ups are never overwritten.

For example, the first follow-up on PR #123 by `octocat` in `NPXInnovation/echo` writes `~/Documents/pr-skills/reviews/PR_REVIEW_echo_123_octocat_addressed_1.txt`.

Use this format:

```
PR #<number> REVIEW FOLLOW-UP <n>
=================================
Review: <review id> by <REVIEWER> on <submitted_at> (<state>), at commit <short commit_id>
Head now: <short headRefOid>, <K> commits since the review
Summary: <A> addressed, <P> partly, <N> not addressed, <B> pushed back, <Q> answered, <D> deferred, <C> closed by you


--------------------------------------------------------------------------------
FILE: <full-file-path>
LINE: <line in the current head, or "original <originalLine>" if the code is gone>
STATUS: <status>
THREAD: <first comment url>
--------------------------------------------------------------------------------

YOUR COMMENT:
<the comment as posted>

RESPONSE:
<replies with author names, reactions, and resolved state, or "none">

ASSESSMENT:
<1-3 sentences on what changed and whether it fixes the comment>

SUGGESTED REPLY:
<only when a reply is useful, in the charlesnpx style below; otherwise omit this block>


================================================================================
NEW ISSUES IN FOLLOW-UP CHANGES
================================================================================

--------------------------------------------------------------------------------
FILE: <full-file-path>
LINE: <line-number>
TYPE: <bug | test quality | code quality | nit>
--------------------------------------------------------------------------------

<comment text in charlesnpx style>


================================================================================
STILL OPEN FROM EARLIER REVIEWS
================================================================================

- <file>:<line> <first comment url> <one line>
```

Omit the "NEW ISSUES" and "STILL OPEN" sections when they are empty. Use current head line numbers from the files fetched in Step 3, not diff offsets.

### Reply and comment style

Write suggested replies and new-issue comments in the voice of charlesnpx:

- Short. 1-3 sentences, lowercase, casual grammar.
- Direct. Say what's still wrong or ask the question. No hedging.
- No markdown headers, bold labels, em dashes, pleasantries, or filler.
- Inline code in backticks for symbols.
- Run /humanizer on each suggested reply and comment before writing it.

Examples:

```
still returns `[]` when `riskId` is null, the fix only covers the undefined case
```

```
fair, didn't realize the caller already validates it
```

```
this moved but the test still mocks the old path, so it isn't testing anything
```
