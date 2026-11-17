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
reject() { printf '#!/bin/sh\nexit 1\n' > "$1/hooks/pre-receive"; chmod +x "$1/hooks/pre-receive"; }
allow() { rm -f "$1/hooks/pre-receive"; }

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
  echo '{}' > .ua/domain-graph.json
  echo '{}' > .ua/diff-overlay.json
  echo '{}' > .ua/intermediate/scan-result.json
  echo '{}' > .ua/intermediate/batch-1.json
  echo x > .ua/tmp/x
  echo y > .ua/.trash-1/y
  "$SYNC" setup
  [ "$(git ls-tree -r --name-only origin/understand-graph | tr '\n' ' ')" \
    = ".gitignore .understandignore domain-graph.json intermediate/scan-result.json knowledge-graph.json meta.json " ]
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
  # SKILL.md step 2's discard gets past it to the newer published graph
  "$SYNC" discard
  "$SYNC" pull
  grep -q 3333333 .ua/meta.json
}

push_failure_is_not_a_race() {
  cd "$T/a"
  reject "$T/origin.git"
  meta 6666666 .ua/meta.json
  local c; c=$(code "$SYNC" push incremental)
  allow "$T/origin.git"
  [ "$c" = 6 ]
  grep -q 6666666 .ua/meta.json
  "$SYNC" push incremental
  git fetch -q origin
  [ "$(git log -1 --format=%s origin/understand-graph)" = "Graph at 6666666 (incremental)" ]
}

push_labels_derived_artifacts() {
  cd "$T/a"
  echo '# Onboarding' > .ua/UA_ONBOARDING.md
  "$SYNC" push onboard
  # a branch set up before domain-graph.json joined the allowlist
  grep -v domain-graph .ua/.gitignore > .ua/.gitignore.old && mv .ua/.gitignore.old .ua/.gitignore
  git -C .ua rm -q --cached domain-graph.json
  git -C .ua commit -q -am "old allowlist" && git -C .ua push -q
  echo '{"domains": []}' > .ua/domain-graph.json
  "$SYNC" push domain
  git fetch -q origin
  [ "$(git log -2 --format=%s origin/understand-graph | tr '\n' '|')" \
    = "Graph at 6666666 (domain)|old allowlist|" ]
  git cat-file -e origin/understand-graph:domain-graph.json
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

push_requires_attached() {
  clone f
  mkdir .ua && meta 8888888 .ua/meta.json
  echo code > code.txt && git add code.txt
  local head; head=$(git rev-parse HEAD)
  [ "$(code "$SYNC" push incremental)" = 5 ]
  [ "$(git rev-parse HEAD)" = "$head" ]
}

keep_local_without_ua_publishes_nothing() {
  clone g
  local before; before=$(git rev-parse origin/understand-graph)
  "$SYNC" setup --keep local
  git fetch -q origin
  [ "$(git rev-parse origin/understand-graph)" = "$before" ]
  [ -f .ua/meta.json ]
}

reattach_after_rm() {
  cd "$T/g"
  rm -rf .ua
  "$SYNC" setup
  [ "$("$SYNC" status | cut -d' ' -f1)" = ua=worktree ]
}

unreachable_origin_is_an_error() {
  clone h
  git remote set-url origin "$T/nowhere.git"
  mkdir .ua && meta 9999999 .ua/meta.json
  [ "$(code "$SYNC" setup)" != 0 ]
  [ ! -e .ua/.git ]
  none '.ua.bak-*'
}

failed_first_publish_rolls_back() {
  git init -q --bare -b v2 "$T/origin2.git"
  git clone -q "$T/origin2.git" "$T/e" 2>/dev/null # empty-repo warning
  cd "$T/e"
  printf '.ua\n/.ua.bak-*\n' > .gitignore
  git add .gitignore && git commit -q -m init && git push -q origin HEAD:v2
  mkdir .ua && meta 7777777 .ua/meta.json
  reject "$T/origin2.git"
  local c; c=$(code "$SYNC" setup)
  allow "$T/origin2.git"
  [ "$c" != 0 ]
  [ ! -e .ua/.git ]
  grep -q 7777777 .ua/meta.json
  none '.ua.bak-*'
  [ "$(git worktree list | wc -l)" = 1 ]
  [ -z "$(git branch --list understand-graph)" ]
  "$SYNC" setup
  [ "$("$SYNC" status | cut -d' ' -f1)" = ua=worktree ]
}

for current in setup_publishes_only_allowlist fresh_clone_attaches plain_and_remote_needs_choice \
  push_without_changes_is_noop push_publishes_change push_refused_when_remote_moved \
  push_failure_is_not_a_race push_labels_derived_artifacts pull_refuses_dirty_worktree keep_local_publishes_local \
  push_requires_attached keep_local_without_ua_publishes_nothing reattach_after_rm \
  unreachable_origin_is_an_error failed_first_publish_rolls_back; do
  "$current"
  echo "ok $current"
done
