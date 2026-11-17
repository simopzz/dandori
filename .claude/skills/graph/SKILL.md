---
name: graph
description: Bring the understand-anything knowledge graph up to date with v2 and publish it to the understand-graph branch.
disable-model-invocation: true
---

# Update and publish the knowledge graph

`.ua/` is a git worktree of the public orphan branch `understand-graph`. `sync.sh` (next to this file, run from the repo root) moves the graph between the two. Its header lists each command and exit code. Graph building is the understand-anything plugin's `understand-anything:understand` skill. Every step below runs with the user in the loop: when a step says ask, stop and wait for the answer. Any `sync.sh` exit this file doesn't name: show its output to the user and stop. Exit 7 means origin is unreachable.

`.ua` is a worktree: to start it over, run `git worktree remove .ua`, then step 1.

## 1. Attach

`/graph` runs in the main checkout, because the understand skill always builds into the main checkout's `.ua`. If `git rev-parse --git-dir` differs from `git rev-parse --git-common-dir`, this is a sibling worktree. Tell the user to run `/graph` from the main checkout, and stop.

Run `.claude/skills/graph/sync.sh status`. Unless it reports `ua=worktree`, run `sync.sh setup`:

- **Exit 2:** a local graph and a published one both exist. Show the user both shas from the status line (`local=`, `published=`), ask which to keep, then rerun `sync.sh setup --keep local` or `--keep remote`.
- **Exit 0:** `.ua` is attached. Any previous plain `.ua` was moved to `.ua.bak-<timestamp>`. Name that directory to the user. It is theirs to delete.
- **Exit 4 or 6 on a first setup:** publishing failed. Setup has already put `.ua` back as it was.

Done when `status` reports `ua=worktree`.

## 2. Preflight

1. Run `git fetch origin`. The graph tracks `v2`, so the checkout must be on branch `v2`, at `origin/v2`, with a clean tree (`git status --porcelain` empty). If any of the three fails, tell the user which. Offer `git switch v2 && git pull --ff-only` only when the tree is clean; otherwise ask how to proceed.
2. Run `sync.sh pull`. On exit 3, `.ua` holds uncommitted graph files, for example a graph a refused push left behind. Show the `git -C .ua status --short` output and ask whether to discard them (`sync.sh discard && sync.sh pull`) or stop.

Done when the checkout is `v2` at `origin/v2` and `sync.sh pull` exited 0.

## 3. Update

Invoke `understand-anything:understand` with no arguments, and relay its questions to the user as it asks them.

A full analysis costs roughly 1.2–1.5M tokens, so it starts only on the user's yes. When `.ua/intermediate/incremental-plan.json` says `FULL_UPDATE`, or the understand skill is about to run its full pipeline for any other reason (no graph yet), stop before its Phase 0.5. Tell the user it is a full rebuild and why (the plan's `filesToReanalyze` count), with that cost. Ask whether to go ahead. On no, stop: the published graph is unchanged.

When it reaches its symbol-loss gate, at the merge or at the save, step 4 decides before `prepare-symbol-retry.mjs` runs. A STOP inside the understand skill ends that skill, not `/graph`: continue at step 4 for a gate, or step 5 once `meta.json` has advanced.

## 4. Blocks

When the merge or `finalize-incremental.mjs` reports `unresolvedFiles`, read `.ua/intermediate/incremental-symbol-report.json`. Take every `missing[]` entry in the unresolved files whose `status` is not `deleted`:

- **All `still-present`:** the analyzer dropped symbols that still exist, which its one retry can repair. Let the understand skill run that retry. If it blocks again, go to the next branch.
- **Anything else** (`unknown`: parser limits, reused IDs, ambiguity): a retry reproduces the block. List each entry as `file: name (reason)`. Ask whether to run `understand-anything:understand --full`, a full rebuild of roughly 1.2–1.5M tokens.
  - **Yes:** run it, then go to step 5.
  - **No:** stop. Tell the user both the published graph and `.ua/meta.json` are unchanged.

Other `understand` failures go to the user as they stand, with no rebuild offer.

## 5. Publish

Publish only a graph of this checkout: `gitCommitHash` in `.ua/meta.json` must equal `git rev-parse HEAD`. If it doesn't, say so and stop.

Run `sync.sh push full` after any full analysis, otherwise `sync.sh push incremental`:

- **`nothing to publish`:** the published graph already matches.
- **Exit 4:** another machine published first. The refused graph waits as uncommitted files in `.ua`. Rerunning `/graph` pulls the newer graph, and step 2 asks what to do with these files.
- **Exit 6:** the push failed (network, auth), and `origin/understand-graph` didn't move. The graph is still staged in `.ua`. Tell the user, and offer to rerun the same `sync.sh push` command.

Publishing always goes through `sync.sh push`, which never force-pushes.

## 6. Derived artifacts

Two artifacts are built from the graph and go stale when it changes: the domain graph (`.ua/domain-graph.json`, the dashboard's domain view) and the onboarding guide (`.ua/UA_ONBOARDING.md`). After a publish that changed the graph, ask once which of them to regenerate: domain graph, onboarding guide, both, or neither. Each one is a separate LLM pass, so neither is the default. Run each one the user picks, then publish it:

- **Domain graph:** invoke `understand-anything:understand-domain` with no arguments. It derives from the graph in `.ua` and saves `.ua/domain-graph.json`. Then run `sync.sh push domain`.
- **Onboarding guide:** invoke `understand-anything:understand-onboard`. Where it offers to save to `docs/UA_ONBOARDING.md`, write the guide to `.ua/UA_ONBOARDING.md` instead, without asking. The guide is published with the graph, not committed to the code repo. Then run `sync.sh push onboard`.

The push exits are the same as in step 5. Finish with one line: the last published commit (`git -C .ua log -1 --format='%h %s'`) and `https://github.com/simopzz/dandori/tree/understand-graph`.
