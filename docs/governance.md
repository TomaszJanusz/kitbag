# Repository governance

Repository files describe policy; GitHub enforces it only after the maintainer
completes the setup below. KIT-10 does not apply settings or install apps.
As checked on 2026-10-24, squash-only merging, branch deletion after merge and
auto-merge are enabled; no ruleset protects `main` yet.

## Main ruleset

[`rulesets/main.json`](rulesets/main.json) is the request body for GitHub's
[Create a repository ruleset API](https://docs.github.com/en/rest/repos/rules#create-a-repository-ruleset).
Its fields were compared with that API's documented request shape and its JSON
syntax checked locally. No server-side import has been attempted.

The active branch ruleset targets exactly `refs/heads/main`, with no bypass
actors (including administrators and Renovate). It blocks deletion and force
pushes, requires linear history and a PR, and permits only squash merging.
The PR requirement blocks direct pushes; an `update` restriction is deliberately
absent because with no bypass actors it would also prevent ordinary PR merges.

Required checks, bound to the GitHub Actions app (integration ID `15368`), are:

- `ci-success`: the existing aggregate requires `check (ubuntu-latest)`,
  `check (macos-latest)`, `windows`, `modules` and `security` to succeed.
- `pr-title`: the separate title workflow must succeed too. Do not omit it:
  `ci-success` does not depend on this workflow.

Branches must be up to date before merge and review threads must be resolved.
Stale approvals are dismissed on new pushes. Required approvals are **zero**;
code-owner approval and last-push approval are disabled because GitHub does not
let the solo maintainer approve his own PRs. CODEOWNERS still requests review.
This trades mandatory independent review for a usable solo-maintainer workflow;
when a second maintainer joins, reconsider requiring one approval. Empty bypass
actors do not prevent a repository administrator from editing the ruleset itself.

## Maintainer setup (after merging this PR)

1. Confirm `ci-success` and `pr-title` have run successfully and the existing
   squash-only, delete-on-merge and auto-merge settings remain enabled:

   ```sh
   gh api repos/TomaszJanusz/kitbag \
     --jq '{allow_squash_merge,allow_merge_commit,allow_rebase_merge,delete_branch_on_merge,allow_auto_merge}'
   gh api repos/TomaszJanusz/kitbag/rulesets
   ```

2. From the updated default branch, create the ruleset **once** with the exact
   checked-in body (requires repository Administration write permission):

   ```sh
   gh api --method POST repos/TomaszJanusz/kitbag/rulesets \
     -H 'Accept: application/vnd.github+json' \
     -H 'X-GitHub-Api-Version: 2026-03-10' \
     --input docs/rulesets/main.json
   gh api repos/TomaszJanusz/kitbag/rules/branches/main
   ```

   Save the returned ruleset ID. For future changes, review the file first and
   use `gh api --method PUT repos/TomaszJanusz/kitbag/rulesets/RULESET_ID
   --input docs/rulesets/main.json` instead of creating duplicates. Check the
   Settings → Rules → Rulesets UI: active, no bypass, both checks required.
   Verify a draft PR with a bad title fails `pr-title`; after marking it ready,
   the merge box must remain blocked until the title is corrected. Do not merge
   a negative control or attempt a direct push to test production protection.

3. Enable private vulnerability reporting in Settings → Security → Advanced
   Security → Private vulnerability reporting → Enable (the location may also
   appear under Code security). Alternatively, the maintainer can run:

   ```sh
   gh api --method PUT repos/TomaszJanusz/kitbag/private-vulnerability-reporting
   gh api repos/TomaszJanusz/kitbag/private-vulnerability-reporting
   ```

   Check that the [private report form](https://github.com/TomaszJanusz/kitbag/security/advisories/new)
   is available. See [SECURITY.md](../SECURITY.md) for reporting expectations.

4. Install the [Renovate GitHub App](https://github.com/apps/renovate) for **only
   this repository**. Review its permissions and onboarding output. Because
   `renovate.json` already exists, the app may treat the repository as already
   onboarded and skip an onboarding PR; record that outcome rather than claiming
   a PR was opened. If the installation offers onboarding, review its PR.
   Confirm the first grouped Go-module or GitHub-Actions update is opened; use
   the Dependency Dashboard to request it outside the schedule if needed.
   An update requires an available upstream change. Onboarding and the first
   grouped update remain pending until the app runs.

5. Verify the Scorecard workflow triggered by the merge into `main`, then check
   the weekly run, Security → Code scanning SARIF results, and the README badge.
   Inspect the analysis log for successful **publication**, since v2.5.0 can
   warn about a publication failure without failing the scan. Scheduled workflows
   run only from the default branch. No pre-merge run proves publication.

## Dependency updates

`renovate.json` enables the Go modules, GitHub Actions and mise managers. Go
modules and Actions are grouped separately, with patch updates separated from
minor updates; mise tools have their own group. Actions are pinned to full commit
digests and those pins are updated. Digest-only, pin, minor and major updates
require manual merge; only patch updates opt into automerge. Renovate uses
Conventional Commit titles, the `dependencies` label and a Monday-before-07:00
Europe/Warsaw schedule. Its default dashboard provides manual scheduling controls.

Automerge uses PRs and squash, with `platformAutomerge: false` and
`ignoreTests: false`: Renovate waits for checks rather than queuing platform
automerge before checks exist. The installed ruleset is what explicitly requires
`ci-success` and `pr-title`; **apply it before installing Renovate**. There is no
Renovate bypass. Do not use `statusCheckNames` to name required CI checks; that
option only renames Renovate's own statuses.

The [mise manager](https://docs.renovatebot.com/modules/manager/mise/) supports
tools in `mise.toml`, including supported `core:`, `aqua:` and `go:` backends.
Check extraction in the app's dashboard/logs; unsupported backends or version
schemes need manual updates. Tool versions stay pinned in `mise.toml`.
The configuration has not been run through `renovate-config-validator`: this
repository has no pinned Node/Renovate runtime, and no unpinned validator was
installed. JSON syntax was checked; app extraction and behavior remain unverified.

## Workflows and verification

`pr-title` runs on PR open, title edits, pushes, reopen and ready-for-review.
Its shell regex accepts the types listed in [CONTRIBUTING.md](../CONTRIBUTING.md),
optional scopes such as `feat(manifest):`, and optional breaking-change `!`.
The title is passed through an environment variable with no checkout or token
permissions. Newlines and empty descriptions fail.

The isolated draft [PR #12](https://github.com/TomaszJanusz/kitbag/pull/12) proved
the check on GitHub: the non-conventional title
[failed](https://github.com/TomaszJanusz/kitbag/actions/runs/40135822846/job/120997001848),
then the corrected title
[passed](https://github.com/TomaszJanusz/kitbag/actions/runs/40135864263/job/120997124047).
It was closed without merging; `throwaway/kit-10-bad-title` is retained. Actual
merge blocking remains pending installation of the required-check ruleset.

`scorecard.yml` runs Mondays at 06:23 UTC and on pushes to `main`, publishes
results and uploads SARIF. Its job has only contents-read, security-events-write
and id-token-write, with no PAT. It follows the
[Scorecard publishing restrictions](https://github.com/ossf/scorecard-action#workflow-restrictions):
no workflow/job environment or defaults, no workflow write permissions, and only
approved actions in the analysis job.

Action pins were resolved using read-only `gh api` calls to
`repos/OWNER/REPO/git/ref/tags/TAG`, then `git/tags/TAG_OBJECT_SHA` for annotated
tags until a commit was reached:

- `actions/checkout` v7.0.1 → `3d3c42e5aac5ba805825da76410c181273ba90b1`.
- `ossf/scorecard-action` v2.5.0 → `fa7117db83c52d787ae5a89f350999701505d78e`.
- `github/codeql-action` v4.37.4 → `f205ea1c3313d32999d8d6a48b4f6530d4437b38`.

Actionlint and zizmor audit all workflows locally and in the existing security
CI job. Scorecard is intentionally not required on PRs; it publishes from main.
