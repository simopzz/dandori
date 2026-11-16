#!/usr/bin/env bash
# Self-test for sync.sh: a throwaway bare origin and clones under mktemp -d.
# Run: bash .claude/skills/graph/sync_test.sh
set -eEuo pipefail

SYNC="$(cd "$(dirname "$0")" && pwd)/sync.sh"
T=$(mktemp -d)
trap '[ -n "${KEEP:-}" ] || rm -rf "$T"' EXIT
trap 'echo "FAIL ${current:-setup} (line $LINENO)" >&2' ERR
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t

sha() { printf '%s%.0s' "$1" {1..5}; }               # 7 chars ×5 = a 35-char fake hash
meta() { printf '{"gitCommitHash": "%s"}\n' "$(sha "$1")" > "$2"; }
none() { ! compgen -G "$1" >/dev/null; }
code() { local c=0; "$@" >/dev/null 2>&1 || c=$?; echo "$c"; }
clone() { git clone -q "$T/origin.git" "$T/$1"; cd "$T/$1"; }

git init -q --bare -b v2 "$T/origin.git"
git clone -q "$T/origin.git" "$T/a" 2>/dev/null # empty-repo warning
cd "$T/a"
printf '.ua\n/.ua.bak-*\n' > .gitignore
git add .gitignore && git commit -q -m init && git push -q origin HEAD:v2

setup_publishes_only_allowlist() {
  cd "$T/a"
  mkdir -p .ua/intermediate .ua/tmp .ua/.trash-1
  meta abcdef1 .ua/meta.json
  echo '{}' > .ua/knowledge-graph.json
  echo '*.templ' > .ua/.understandignore
  echo '{}' > .ua/intermediate/scan-result.json
  echo '{}' > .ua/intermediate/batch-1.json
  echo x > .ua/tmp/x
  echo y > .ua/.trash-1/y
  "$SYNC" setup
  [ "$(git ls-tree -r --name-only origin/understand-graph | tr '\n' ' ')" \
    = ".gitignore .understandignore intermediate/scan-result.json knowledge-graph.json meta.json " ]
  [ -f "$(compgen -G '.ua.bak-*')/tmp/x" ]
  [ "$("$SYNC" status)" = "ua=worktree remote=yes local=abcdef1 published=abcdef1" ]
  [ "$(git -C .ua log -1 --format=%s)" = "Graph at abcdef1 (setup)" ]
  [ -z "$(git status --porcelain)" ]
}

fresh_clone_attaches() {
  clone b
  [[ "$("$SYNC" status)" == "ua=none remote=yes"* ]]
  "$SYNC" setup
  [ -f .ua/knowledge-graph.json ]
}

plain_and_remote_needs_choice() {
  clone c
  mkdir .ua && meta 1111111 .ua/meta.json
  [ "$(code "$SYNC" setup)" = 2 ]
  [ ! -e .ua/.git ]
  grep -q 1111111 .ua/meta.json
  none '.ua.bak-*'
  "$SYNC" setup --keep remote
  grep -q abcdef1 .ua/meta.json
  grep -q 1111111 "$(compgen -G '.ua.bak-*')/meta.json"
}

push_without_changes_is_noop() {
  cd "$T/a"
  local before; before=$(git rev-list --count origin/understand-graph)
  [ "$("$SYNC" push incremental)" = "nothing to publish" ]
  git fetch -q origin
  [ "$(git rev-list --count origin/understand-graph)" = "$before" ]
}

push_publishes_change() {
  cd "$T/a"
  meta 2222222 .ua/meta.json
  "$SYNC" push incremental
  git fetch -q origin
  [ "$(git log -1 --format=%s origin/understand-graph)" = "Graph at 2222222 (incremental)" ]
}

push_refused_when_remote_moved() {
  cd "$T/b"
  "$SYNC" pull
  meta 3333333 .ua/meta.json
  "$SYNC" push full
  cd "$T/a"
  meta 4444444 .ua/meta.json
  [ "$(code "$SYNC" push full)" = 4 ]
  # the refused graph stays as uncommitted changes, so the next pull stops and asks
  git -C .ua fetch -q origin
  [ -z "$(git -C .ua rev-list origin/understand-graph..HEAD)" ]
  grep -q 4444444 .ua/meta.json
  [ "$(code "$SYNC" pull)" = 3 ]
}

pull_refuses_dirty_worktree() {
  cd "$T/b"
  echo changed > .ua/knowledge-graph.json
  [ "$(code "$SYNC" pull)" = 3 ]
  grep -q changed .ua/knowledge-graph.json
}

keep_local_publishes_local() {
  clone d
  mkdir .ua && meta 5555555 .ua/meta.json
  "$SYNC" setup --keep local
  git fetch -q origin
  [ "$(git log -1 --format=%s origin/understand-graph)" = "Graph at 5555555 (setup)" ]
  # files the local graph lacks don't survive from the published one
  [ "$(git ls-tree -r --name-only origin/understand-graph | tr '\n' ' ')" = ".gitignore meta.json " ]
}

for current in setup_publishes_only_allowlist fresh_clone_attaches plain_and_remote_needs_choice \
  push_without_changes_is_noop push_publishes_change push_refused_when_remote_moved \
  pull_refuses_dirty_worktree keep_local_publishes_local; do
  "$current"
  echo "ok $current"
done
