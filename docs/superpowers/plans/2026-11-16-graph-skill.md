# `/graph` Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A `/graph` project skill that pulls the published knowledge graph, updates it with `/understand`, handles symbol-gate blocks by asking before any full rebuild, and pushes the result to the orphan branch `understand-graph`.

**Architecture:** The git plumbing lives in a small deterministic script, `.claude/skills/graph/sync.sh`, with a self-test. `SKILL.md` sequences it around `/understand-anything:understand` and holds the judgement parts: the `v2` preflight and the block policy. `.ua/` becomes a git worktree of `understand-graph`.

**Tech Stack:** bash and git ≥ 2.42 (`worktree add --orphan`; local git is 2.56), plus the understand-anything plugin skill.

**Spec:** `docs/superpowers/specs/2026-11-16-graph-skill-design.md`

## Global Constraints

- Branch name: `understand-graph`. Worktree path: `.ua` at the repo root.
- Published files, an allowlist: `knowledge-graph.json`, `meta.json`, `fingerprints.json`, `config.json`, `.understandignore`, `UA_ONBOARDING.md`, `intermediate/scan-result.json`, plus the branch's own `.gitignore`.
- Commit message: `Graph at <first 7 chars of meta.json gitCommitHash> (<incremental|full|setup>)`. No AI attribution lines.
- Never force-push. Never delete a user's `.ua` contents: move them to `.ua.bak-<unix-ts>` instead.
- Never start `/understand --full` without the user's explicit yes.
- Work happens on a branch off `v2`, merged by PR (AGENTS.md). Run `mise run check` before each commit.

## Review Focus

- **`.ua` already holds a graph and the remote branch also has one.** Setup must stop without changing anything, show both graphs' commits, and continue only with `--keep local|remote` (Task 1 test `plain_and_remote_needs_choice`).
- **Leftovers from an aborted `/understand` run** (`tmp/`, `.trash-*`, `intermediate/batch-*.json`) must never be published (Task 1 test `setup_publishes_only_allowlist`).
- **Running `/graph` twice with nothing changed** must not create an empty commit (Task 1 test `push_without_changes_is_noop`).
- **Another machine published first.** The push must be refused without forcing, exit 4, and the skill says to rerun (Task 1 test `push_refused_when_remote_moved`).
- **The branch picks up a hand edit, or a crash leaves tracked files modified.** `pull` refuses and exits 3 instead of discarding or merging them (Task 1 test `pull_refuses_dirty_worktree`).

---

### Task 1: `sync.sh`, the git plumbing for the graph branch, with a self-test

**Files:**
- Create: `.claude/skills/graph/sync.sh` (executable)
- Create: `.claude/skills/graph/sync_test.sh` (executable)
- Modify: `.gitignore`: add `!.claude/skills/graph` after the `!.claude/skills/grafana-debugging` line

**Interfaces:**
- Produces: `sync.sh <command>`, run with the repo root as the working directory:
  - `status` prints one line: `ua=<worktree|plain|none> remote=<yes|no>`, plus ` local=<sha7|-> published=<sha7|->` (the `gitCommitHash` of `.ua/meta.json` and of `origin/understand-graph:meta.json`). Exit 0.
  - `setup [--keep local|remote]`:
    - `ua=worktree`: no-op, exit 0.
    - `remote=no`: move `.ua` (if present) to `.ua.bak-<ts>` and run `git worktree add --orphan -b understand-graph .ua`. Write the allowlist `.gitignore`, copy the backup's files in, commit with `(setup)` and `git push -u origin understand-graph`.
    - `remote=yes`, `ua=none`: worktree-add `.ua` tracking `origin/understand-graph`.
    - `remote=yes`, `ua=plain`, no `--keep`: print the `status` line and exit 2, touching nothing. With `--keep remote`: back up `.ua`, then worktree-add. With `--keep local`: back up `.ua`, worktree-add, copy the backup's files over, commit `(setup)` and push.
  - `pull`: requires `ua=worktree`, else exit 5. If tracked files are modified, exit 3. Otherwise `git -C .ua pull --ff-only`.
  - `push <incremental|full>`: requires `ua=worktree`. Stage everything (the allowlist `.gitignore` filters it). With nothing staged, print `nothing to publish` and exit 0. Otherwise commit and push. A refused push exits 4 and leaves the local commit in place.
- The branch `.gitignore` content (an allowlist, so unknown files never publish):
  ```
  *
  !.gitignore
  !knowledge-graph.json
  !meta.json
  !fingerprints.json
  !config.json
  !.understandignore
  !UA_ONBOARDING.md
  !intermediate/
  intermediate/*
  !intermediate/scan-result.json
  ```

- [ ] **Step 1: Write `sync_test.sh`.** It builds a throwaway world under `mktemp -d`: a bare `origin.git`, clone A with one commit on `v2`, and a fake `.ua/`. The fake `.ua/` holds `meta.json` (`{"gitCommitHash":"abcdef1234..."}`), `knowledge-graph.json`, `.understandignore`, `intermediate/scan-result.json`, `intermediate/batch-1.json`, `tmp/x` and `.trash-1/y`. Each case is a bash function that `set -e`-fails on a broken assertion. The runner prints `ok <name>` per case and exits non-zero on the first failure. Cases and assertions:
  - `setup_publishes_only_allowlist`: in A, `sync.sh setup` exits 0. `git ls-tree -r --name-only origin/understand-graph` equals exactly `.gitignore .understandignore intermediate/scan-result.json knowledge-graph.json meta.json`. `.ua.bak-*/tmp/x` exists. `status` prints `ua=worktree remote=yes local=abcdef1 published=abcdef1`. The latest commit subject is `Graph at abcdef1 (setup)`.
  - `fresh_clone_attaches`: clone B, where `status` starts with `ua=none remote=yes`. `setup` exits 0 and `.ua/knowledge-graph.json` exists.
  - `plain_and_remote_needs_choice`: clone C plus a plain `.ua/meta.json` holding `1111111…`. `setup` exits 2. `.ua` is still a plain directory with unchanged contents, and no `.ua.bak-*` exists. Then `setup --keep remote` exits 0, `.ua/meta.json` shows `abcdef1`, and a `.ua.bak-*` holds `1111111`.
  - `push_without_changes_is_noop`: in A, `push incremental` prints `nothing to publish` and the remote branch's commit count is unchanged.
  - `push_publishes_change`: in A, set `.ua/meta.json` to `2222222…`. `push incremental` exits 0, and the remote's latest subject is `Graph at 2222222 (incremental)`.
  - `push_refused_when_remote_moved`: B changes `meta.json` to `3333333…` and pushes. A changes it to `4444444…`, and `push full` exits 4.
  - `pull_refuses_dirty_worktree`: in B, edit the tracked `.ua/knowledge-graph.json`, and `pull` exits 3.

- [ ] **Step 2: Run it and watch it fail.** Run `bash .claude/skills/graph/sync_test.sh`. Expected: it fails on the first case because `sync.sh` doesn't exist.

- [ ] **Step 3: Implement `sync.sh`** to the interface above. Use `set -euo pipefail`. Read the meta sha with `sed -n 's/.*"gitCommitHash": *"\([0-9a-f]\{7\}\).*/\1/p'` so it doesn't depend on `jq`. Use `git ls-remote --exit-code --heads origin understand-graph` for `remote=`. `ua=worktree` means `.ua/.git` exists and `git -C .ua rev-parse --abbrev-ref HEAD` is `understand-graph`.

- [ ] **Step 4: Run it and watch it pass.** Run `bash .claude/skills/graph/sync_test.sh`. Expected: 7 `ok` lines, exit 0.

- [ ] **Step 5: Commit** on the feature branch `graph-skill`, created off `origin/v2`; there's no issue number. Run `mise run check` first.
  ```bash
  git add .gitignore .claude/skills/graph/sync.sh .claude/skills/graph/sync_test.sh
  git commit -m "Add the graph branch sync script"
  ```

### Task 2: `SKILL.md`, the `/graph` playbook

**Files:**
- Create: `.claude/skills/graph/SKILL.md`

**Interfaces:**
- Consumes: `sync.sh status|setup|pull|push` and their exit codes from Task 1.
- Produces: the user-invocable skill `graph`.

- [ ] **Step 1: Write `SKILL.md`.** The frontmatter has `name: graph` and a description along the lines of "Use when the user wants the understand-anything knowledge graph brought up to date and published, or types /graph". The body is numbered steps, written per `mattpocock-skills:writing-for-agents`, which you load first:
  1. **Attach.** Run `sync.sh status`. If it isn't `ua=worktree`, run `setup`. On exit 2, show both shas and ask which graph to keep, then rerun with `--keep`.
  2. **Preflight.** `git fetch origin`. If the checkout isn't `v2` at `origin/v2` with a clean tree, say which of the three conditions fails, and offer `git switch v2 && git pull --ff-only` only when the tree is clean. Then `sync.sh pull`: on exit 3, show `git -C .ua status --short` and ask before doing anything.
  3. **Update.** Invoke `understand-anything:understand` with no arguments, and let it ask its own questions.
  4. **Blocks.** This step overrides the understand skill's retry step. When its merge reports `unresolvedFiles`, read `.ua/intermediate/incremental-symbol-report.json` first:
     - If every `missing[]` entry in the unresolved files has `status` `still-present`, let it run its one retry.
     - Otherwise, list each non-`deleted` entry as `file: name (reason)`. Say a retry can't fix these, and ask whether to run `/understand-anything:understand --full`, which costs about 1.2–1.5M tokens.
     - If a retry fails, ask the same question.
     - On no: stop, and say the published graph is unchanged.
     - Other `/understand` failures are reported as they are, with no rebuild offer.
  5. **Publish.** Only when `.ua/meta.json`'s `gitCommitHash` equals `git rev-parse HEAD`, run `sync.sh push incremental`, or `push full` if `--full` ran. On exit 4, say another machine published first, and that rerunning `/graph` will pull its graph and build on it. End with one line: the commit published and the `understand-graph` branch URL.
  - Also state, in one line each: the skill depends on the understand-anything plugin; `.ua/` is a worktree of `understand-graph`, so never `rm -rf` it; and the branch is public.

- [ ] **Step 2: Check it against the spec.** Every bullet under the spec's "Each run" maps to a step above. Check that `rg -n "force" .claude/skills/graph/SKILL.md` finds only the "never force" line.

- [ ] **Step 3: Commit.**
  ```bash
  git add .claude/skills/graph/SKILL.md
  git commit -m "Add the /graph skill"
  ```

### Task 3: First run on this machine, then the PR

**Files:**
- Commit: `docs/superpowers/specs/2026-11-16-graph-skill-design.md`, `docs/superpowers/plans/2026-11-16-graph-skill.md`

- [ ] **Step 1: Run `/graph` for real.** Expected: setup moves the current `.ua` to `.ua.bak-<ts>` and publishes it as `Graph at 8ed2209 (setup)`. The incremental update then blocks on `internal/planning/service.go: refusal`, and the skill asks about `--full`. The user decides.
- [ ] **Step 2: Check the branch.** `git ls-tree -r --name-only origin/understand-graph` lists only allowlisted files.
- [ ] **Step 3: Commit the spec and plan, then open the PR** against `v2` using the `mattpocock-skills:pr` skill.
