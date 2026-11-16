# `/graph`: update and publish the knowledge graph on demand

## Why

The understand-anything knowledge graph (`.ua/`) lives on one machine, gitignored. An incremental `/understand` can block on its symbol-loss gate. That happens with templ, which is now ignored for this reason, with Go files that alias an import, and with a method that turns into a free function. When it blocks, the graph silently goes stale. The user wants one command, run when they choose, that brings the graph up to date, deals with blocks, and publishes the result to GitHub. CI automation was considered and dropped.

## Goals

- One command in Claude Code: pull the published graph, update it, push it.
- A block never leaves the graph silently stale. The command explains it and offers the way through.
- The published branch carries everything the next incremental run needs, so any machine can continue from it.

## Non-goals and anti-goals

- No CI and no headless runs. Nothing runs unless the user starts it.
- It does not pre-answer `/understand`'s own questions. The user stays in the loop.
- It never starts a full rebuild (about 1.2–1.5M tokens) without the user's yes.
- It never force-pushes or overwrites a newer published graph.

## Approaches

- **A, chosen: a project skill, `/graph`.** It wraps pull → `/understand` → push and owns block handling.
- **B: two `mise` tasks (`graph:pull`, `graph:push`), with `/understand` run by hand between them.** Rejected: it takes three steps, and the user still has to spot blocks and decide what to do.
- **Headless `claude -p`: ruled out.** It can't stop and ask.

## Design

**Where the graph is published.** An orphan branch, `understand-graph`. It holds `knowledge-graph.json`, `meta.json`, `fingerprints.json`, `config.json`, `.understandignore`, `intermediate/scan-result.json` and `UA_ONBOARDING.md`. The branch's own `.gitignore` drops `.trash-*/`, `tmp/` and the rest of `intermediate/`.

**`.ua/` is a git worktree of that branch.** `.ua` is already gitignored on the code branches, so the two don't collide. Pulling is `git -C .ua pull --ff-only`. Publishing is a commit and push in `.ua`.

**First-run setup, done by the skill when `.ua` is not yet that worktree:**
- If `origin/understand-graph` is missing, create the orphan branch from the current `.ua/` contents. Move the old directory to a timestamped backup next to it; don't delete it.
- If the branch exists and a plain `.ua/` directory is also present, show which commit each graph describes and ask which one to keep before replacing anything.

**Each run:**
1. *Preflight.* Fetch. If `.ua` has uncommitted changes, ask before doing anything. Pull the branch with `--ff-only`. Check that the checkout is `v2` at `origin/v2` with a clean tree. If it isn't, say so and offer to switch. (My call: the published graph tracks the default branch, which is what the original CI idea was about.)
2. *Update.* Invoke `/understand-anything:understand` and let it ask its questions as usual.
3. *Blocks.* When the incremental merge reports `unresolvedFiles`, read `incremental-symbol-report.json` before any retry:
   - If every missing symbol is `still-present`, meaning the analyzer left out something that still exists, let `/understand` use its one retry.
   - Otherwise (`unknown` entries, from parser limits or reused IDs), skip the retry, which can't fix these. Report each blocked symbol and the reason, then ask whether to run `--full`.
   - If the retry itself fails, ask about `--full` the same way.
   - Declining leaves both the published graph and the local one untouched.
4. *Publish.* Only after `meta.json` has advanced, commit in `.ua` with a message like `Graph at <short sha> (incremental|full)`, then push. Commit messages carry no AI attribution. If the push is refused because the remote moved, stop and report; don't force.

**Where it lives.** `.claude/skills/graph/SKILL.md`, committed, which needs a `!.claude/skills/graph` line in `.gitignore`, the way `grafana-debugging` has one. The skill notes that it depends on the understand-anything plugin.

## Left to the builder

- The exact wording of the skill.
- How the skill tells an incremental block apart from other `/understand` failures. Other failures are reported as they are, with no rebuild offer.
