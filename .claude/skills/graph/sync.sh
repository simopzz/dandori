#!/usr/bin/env bash
# Keeps .ua/ (the understand-anything knowledge graph) as a git worktree of the
# orphan branch understand-graph, and pulls and publishes it. Driven by SKILL.md.
#
#   sync.sh status                       ua=<worktree|plain|none> remote=<yes|no> local=<sha7|-> published=<sha7|->
#   sync.sh setup [--keep local|remote]  attach .ua to the branch (exit 2: both a local and a published graph, choose)
#   sync.sh pull                         fast-forward .ua (exit 3: uncommitted changes, exit 5: not attached)
#   sync.sh discard                      drop uncommitted graph files in .ua
#   sync.sh push <incremental|full|onboard|domain>  commit and push .ua (exit 4: the published branch moved, 6: push failed otherwise)
# Any command: exit 7 when origin can't be reached.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

BRANCH=understand-graph
UA=.ua
FILES=(knowledge-graph.json meta.json fingerprints.json config.json .understandignore UA_ONBOARDING.md domain-graph.json
  intermediate/scan-result.json)

sha7() { sed -n 's/.*"gitCommitHash": *"\([0-9a-f]\{7\}\).*/\1/p' | head -1; }
local_sha() { { [ -f "$UA/meta.json" ] && sha7 < "$UA/meta.json"; } || true; }
# ls-remote --exit-code exits 2 only when the ref is absent; anything else is an unreachable origin.
remote_exists() {
  local c=0
  git ls-remote --exit-code --heads origin "$BRANCH" >/dev/null 2>&1 || c=$?
  case $c in
    0) return 0 ;;
    2) return 1 ;;
    *) echo "cannot reach origin (git ls-remote exit $c)" >&2; exit 7 ;;
  esac
}
fetch_branch() { git fetch -q origin "+refs/heads/$BRANCH:refs/remotes/origin/$BRANCH"; }

ua_state() {
  # symbolic-ref also names the branch before its first commit
  if [ -e "$UA/.git" ] && [ "$(git -C "$UA" symbolic-ref -q --short HEAD 2>/dev/null)" = "$BRANCH" ]; then
    echo worktree
  elif [ -d "$UA" ]; then echo plain
  else echo none; fi
}

backup() { local to; to="$UA.bak-$(date +%s)"; mv "$UA" "$to"; echo "$to"; }

# An allowlist, so scratch (tmp/, .trash-*, batch files) never publishes.
write_gitignore() {
  # Order matters: the file negations come after `intermediate/*`, which they override.
  printf '%s\n' '*' '!.gitignore' '!intermediate/' 'intermediate/*' "${FILES[@]/#/!}" > "$UA/.gitignore"
}

# Replace the published files in .ua with the ones in $1 (absent there, absent here).
replace_from() {
  local f
  for f in "${FILES[@]}"; do
    rm -f "$UA/$f"
    if [ -f "$1/$f" ]; then mkdir -p "$(dirname "$UA/$f")"; cp "$1/$f" "$UA/$f"; fi
  done
}

cmd_status() {
  local remote=no published=
  if remote_exists; then
    remote=yes
    fetch_branch
    published=$(git show "origin/$BRANCH:meta.json" 2>/dev/null | sha7) || true
  fi
  local l; l=$(local_sha)
  echo "ua=$(ua_state) remote=$remote local=${l:--} published=${published:--}"
}

cmd_push() {
  attached || return # else `git -C .ua` resolves to the code repo
  write_gitignore # a branch set up under an older allowlist picks up new entries
  git -C "$UA" add -A
  if git -C "$UA" diff --cached --quiet; then echo "nothing to publish"; return 0; fi
  local s; s=$(local_sha)
  # --no-verify: the repo's pre-commit hook checks code, not graph data.
  git -C "$UA" commit -q --no-verify -m "Graph at ${s:--} ($1)"
  if ! git -C "$UA" push -q -u origin "$BRANCH"; then
    # Undo the commit so .ua stays on the published history; the graph waits as uncommitted changes.
    git -C "$UA" reset -q --soft HEAD~1 2>/dev/null || git -C "$UA" update-ref -d HEAD
    if fetch_branch 2>/dev/null && ! git -C "$UA" merge-base --is-ancestor "origin/$BRANCH" HEAD 2>/dev/null; then
      echo "push refused: origin/$BRANCH moved. This graph is kept as uncommitted changes in $UA." >&2
      return 4
    fi
    echo "push failed, origin/$BRANCH unchanged. This graph is kept staged in $UA: rerun sync.sh push $1." >&2
    return 6
  fi
}

cmd_setup() {
  local keep=
  if [ "${1:-}" = --keep ]; then keep=${2:-}; fi
  local ua; ua=$(ua_state)
  [ "$ua" = worktree ] && return 0
  git worktree prune # forget a .ua that was deleted by hand
  local bak=
  if ! remote_exists; then
    [ "$ua" = plain ] && bak=$(backup)
    git worktree add -q --orphan -b "$BRANCH" "$UA"
    write_gitignore
    [ -n "$bak" ] && replace_from "$bak"
    local c=0
    cmd_push setup || c=$?
    if [ "$c" != 0 ]; then
      # Undo the attach, so .ua is as it was and the next setup starts over.
      git worktree remove --force "$UA"
      git branch -D "$BRANCH" >/dev/null 2>&1 || true
      [ -z "$bak" ] || mv "$bak" "$UA"
    fi
    return "$c"
  fi
  fetch_branch
  if [ "$ua" = plain ]; then
    case "$keep" in
      local | remote) bak=$(backup) ;;
      *) cmd_status; echo "both a local and a published graph exist: rerun with --keep local or --keep remote" >&2; return 2 ;;
    esac
  fi
  git worktree add -q --track -B "$BRANCH" "$UA" "origin/$BRANCH"
  # --keep local means something only when there was a local graph to keep
  if [ "$keep" = local ] && [ -n "$bak" ]; then replace_from "$bak"; cmd_push setup; fi
}

cmd_pull() {
  attached || return
  if [ -n "$(git -C "$UA" status --porcelain --untracked-files=no)" ]; then
    git -C "$UA" status --short --untracked-files=no >&2
    return 3
  fi
  git -C "$UA" pull -q --ff-only
}

attached() { [ "$(ua_state)" = worktree ] || { echo "$UA is not attached to $BRANCH: run setup" >&2; return 5; }; }

# Drop uncommitted graph files (staged too: a refused push leaves its graph staged).
cmd_discard() { attached || return; git -C "$UA" reset -q --hard; }

usage() { sed -n '5,10p' "$0" >&2; exit 64; }

case "${1:-}" in
  status) cmd_status ;;
  setup) shift; cmd_setup "$@" ;;
  pull) cmd_pull ;;
  discard) cmd_discard ;;
  push) case "${2:-}" in incremental | full | onboard | domain) cmd_push "$2" ;; *) usage ;; esac ;;
  *) usage ;;
esac
