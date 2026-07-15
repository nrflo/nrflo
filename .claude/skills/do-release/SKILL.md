---
name: do-release
description: Cut a new nrflo release. Bumps VERSION, generates human-readable release notes from git log, tags, pushes, monitors the GoReleaser and Docker CI runs, then replaces the GitHub Release body with the synthesized notes. Accepts one SemVer bump argument: `patch` (+0.0.1), `minor` (+0.1.0), `major` (+1.0.0).
---

# /do-release

Cuts a new nrflo release end-to-end. Invoked as:

```
/do-release patch
/do-release minor
/do-release major
```

The argument is **required**. If missing or not one of `patch|minor|major`, stop and tell the user the valid options.

## Step 0 — Confirm the pre-release gate has been run

**Before doing anything else**, ask the user whether they've already run `/pre-release` against the commits about to be tagged. The `/pre-release` skill runs the full manual_testing harness across every supported provider × mode combination and is the only gate that exercises the real CLI binaries against real provider credentials. Releases that skip it have historically shipped regressions that no Go test could have caught.

Prompt verbatim:

> Have you already run `/pre-release` for the commits you're about to tag? (yes / no)

- **yes** → ask the user to paste the VERDICT line from that run. If they reply `VERDICT: PASS`, proceed to Step 1. If anything else (PASS with caveats, FAIL, or "I lost it"), stop and tell them to re-run `/pre-release` and come back when it's clean.
- **no** → stop. Tell them to run `/pre-release` first, and only invoke `/do-release` once it reports `VERDICT: PASS`.

Do not offer to run `/pre-release` yourself from inside this skill — it takes ~15–20 minutes (gated by the slowest folder, normally engine at ~13 min) and the user should be aware they're committing to that wall time. They invoke it explicitly.

## What you're automating

- nrflo version lives in two places that must stay in lock-step:
  - `VERSION` file at repo root (plain `MAJOR.MINOR.PATCH`, no `v`). Source of truth for humans and Makefile.
  - Git tag `vMAJOR.MINOR.PATCH`. Source of truth for GoReleaser (`{{ .Version }}`).
- Pushing a `v*` tag triggers two workflows in parallel:
  - `.github/workflows/release.yml` → GoReleaser → GitHub Release + Homebrew tap at `nrflo/homebrew-tap` (darwin binaries).
  - `.github/workflows/docker.yml` → multi-arch build → `ghcr.io/${owner}/nrflo-server:${VERSION}` + `:latest` (linux/amd64,arm64, api-mode).
- GoReleaser writes an auto-generated body. You overwrite it with a richer, human-readable summary via `gh release edit`.
- The container image tag drops the `v` prefix (e.g. `v0.3.6` → `ghcr.io/nrflo/nrflo-server:0.3.6`). This is enforced by the `IMAGE_TAG` rule in the Makefile and by `docker/metadata-action` in `docker.yml`.

## Step 1 — Preflight (abort on any failure)

Run checks and stop at the first failure. Report which check failed and what the user must fix.

```bash
git rev-parse --abbrev-ref HEAD            # must equal "master"
git status --porcelain                     # must be empty
git fetch origin master
gh auth status                             # must succeed
```

Then reconcile local vs. origin/master:

```bash
AHEAD=$(git rev-list --count origin/master..HEAD)
BEHIND=$(git rev-list --count HEAD..origin/master)
```

- `BEHIND > 0` → **abort**. Local is behind origin/master; the user must pull/rebase manually. Do not auto-pull.
- `AHEAD > 0` and `BEHIND == 0` → local is strictly ahead. Show the ahead commits with `git log origin/master..HEAD --oneline` and **auto-push** with `git push origin master`. If the push fails, surface the error and stop. Do not attempt recovery.
- Both `0` → proceed.

Also require at least one commit since the latest tag:
```bash
LATEST_TAG=$(git describe --tags --abbrev=0)
git log ${LATEST_TAG}..HEAD --oneline | wc -l   # must be > 0
```

## Step 2 — Compute next version

```bash
CURRENT=$(cat VERSION)                         # e.g. 0.3.0
LATEST_TAG=$(git describe --tags --abbrev=0)   # e.g. v0.3.0
```

Require `"v${CURRENT}" == "${LATEST_TAG}"`. If they diverge, **abort** with a clear message — the user must resolve manually. Do not auto-repair.

Parse `CURRENT` as `MAJOR.MINOR.PATCH`. Apply the bump:
- `patch` → `MAJOR.MINOR.(PATCH+1)`
- `minor` → `MAJOR.(MINOR+1).0`
- `major` → `(MAJOR+1).0.0`

Call the result `NEW` (e.g. `0.3.1`) and `NEW_TAG` = `v${NEW}`.

## Step 3 — Build human-readable release notes

Collect commits since the latest tag, including body for context:

```bash
git log ${LATEST_TAG}..HEAD --pretty=format:"%H%x09%s%x09%b%x1e"
```

(The `\x1e` record separator keeps multi-line bodies grouped per commit.)

Process each commit:
1. Strip any leading `[nrworkflow-<hash>] ` prefix from the subject.
2. Classify into one of:
   - **Features** — "add", "support", "introduce", "new"
   - **Fixes** — "fix", "resolve", "correct"
   - **Improvements** — "update", "refactor", "improve", "wire", "rename", "restructure"
   - **Documentation** — touches only `*.md` / `README` / `agent_manual.md` (use `git show --stat` when unsure)
   - **Other** — everything else
3. Write a short, user-facing bullet (not the raw subject). Combine clearly-related commits into one bullet.

Produce Markdown with this shape (omit empty sections):

```markdown
## What's new in vX.Y.Z

### Features
- <bullet>

### Fixes
- <bullet>

### Improvements
- <bullet>

### Documentation
- <bullet>

---

### Commits
- `<sha>` <cleaned subject>
- ...

**Install:** `brew update && brew upgrade nrflo`
**Full changelog:** https://github.com/nrflo/nrflo/compare/${LATEST_TAG}...${NEW_TAG}
```

Write to `/tmp/nrflo-release-notes-${NEW_TAG}.md` via the Write tool so both you and the user can see it.

## Step 4 — Confirm, bump, commit, tag, push

Show the user: current version, new version, new tag, and a preview of the notes file. Ask one `AskUserQuestion` with options **"Release vX.Y.Z now"** / **"Cancel"**.

On cancel: stop. Do not touch files or git.

On approval, execute in order. Stop immediately on any failure:

```bash
printf '%s\n' "${NEW}" > VERSION
git add VERSION
git commit -m "Release ${NEW_TAG}"
git push origin master
git tag "${NEW_TAG}"
git push origin "${NEW_TAG}"
```

If `git push origin master` fails (non-fast-forward, hook, etc.), do **not** create the tag. Surface the error and stop.

## Step 5 — Monitor the release workflows

The tag push triggers **two** workflows in parallel: `release.yml` (GoReleaser) and `docker.yml` (multi-arch container). Find both:

```bash
gh run list --workflow=release.yml --limit 5 \
  --json databaseId,headBranch,event,status,conclusion,url,createdAt
gh run list --workflow=docker.yml --limit 5 \
  --json databaseId,headBranch,event,status,conclusion,url,createdAt
```

For each workflow, match the run whose `headBranch == "${NEW_TAG}"` and `event == "push"`. If a run is not yet visible, poll every 60s via `ScheduleWakeup` (up to ~5 minutes) until it appears. Capture both IDs as `RELEASE_ID` and `DOCKER_ID`.

Watch them. `gh run watch` blocks per-run, so watch the slower one (release / GoReleaser) first; the docker run typically finishes around the same time:

```bash
gh run watch "${RELEASE_ID}" --exit-status
gh run watch "${DOCKER_ID}"  --exit-status
```

For long waits between polls, use `ScheduleWakeup` with `delaySeconds` around 270 (stay inside cache TTL) instead of sleeping. Full release builds take several minutes.

On failure of **either** run (`conclusion != "success"`):
- Fetch and show failure logs: `gh run view <failed_id> --log-failed | tail -200`.
- Identify which workflow failed (`release.yml` or `docker.yml`) and surface that clearly.
- Tell the user the tag is already pushed — they must either fix-forward with another release, or delete the tag (`git push --delete origin ${NEW_TAG} && git tag -d ${NEW_TAG}`) + reset `VERSION`. **Do not perform destructive recovery yourself.**
- If only the docker run failed but release.yml succeeded, the GitHub Release / Homebrew tap is already published — note that and let the user decide whether to re-trigger only the docker workflow via `gh workflow run docker.yml --ref "${NEW_TAG}"`.
- Stop.

## Step 6 — Replace the release body and verify the container image

On CI success for **both** runs:

```bash
gh release edit "${NEW_TAG}" --notes-file "/tmp/nrflo-release-notes-${NEW_TAG}.md"
gh release view "${NEW_TAG}" --json url,assets,isDraft,isPrerelease
```

Verify the release:
- `isDraft == false`
- Assets include both macOS archives (`nrflo_${NEW}_darwin_amd64.*`, `nrflo_${NEW}_darwin_arm64.*`) and `checksums.txt`.

Verify the container image manifest exists with both arches:

```bash
OWNER=$(gh repo view --json owner -q .owner.login)
docker buildx imagetools inspect "ghcr.io/${OWNER}/nrflo-server:${NEW}" \
  --format '{{json .Manifest}}' 2>&1 | head -c 800
```

Confirm the response references both `linux/amd64` and `linux/arm64` platforms. If `docker` is unavailable on the host, fall back to:

```bash
gh api "/users/${OWNER}/packages/container/nrflo-server/versions" \
  --jq '.[0:3][] | {name: .metadata.container.tags, created_at}'
```

…and verify the latest version's tags include `${NEW}` and `latest`.

Print a final summary for the user:
- Released tag and release URL.
- Upgrade command (Homebrew): `brew update && brew upgrade nrflo`.
- Version verify: `nrflo --version` should print `${NEW_TAG}`.
- Container image: `docker pull ghcr.io/${OWNER}/nrflo-server:${NEW}` (also tagged `:latest`).
- Run command: `docker run -d -p 6587:6587 -v nrflo-data:/data -e ANTHROPIC_API_KEY=... ghcr.io/${OWNER}/nrflo-server:${NEW}`.

## Hard rules

- **Never** force-push, rewrite history, or delete tags/branches. Those are user-only actions.
- **Never** skip preflight checks, even on retry.
- **Never** edit `.goreleaser.yaml`, `.github/workflows/release.yml`, `.github/workflows/docker.yml`, or the `Dockerfile` from this skill.
- **Never** proceed past Step 4 confirmation without explicit approval.
- If `VERSION` and the latest git tag disagree at Step 2, stop. Do not guess.
