#!/usr/bin/env bash
# Publishes images on the orphan pr-images branch under <pr>/ and prints the <img> tags a
# PR body shows them with: mise run pr-image <pr> <image>... PRs share the branch, so a
# refused push fetches the latest and commits again.
set -euo pipefail
[ $# -ge 2 ] || { echo "usage: mise run pr-image <pr> <image>..." >&2; exit 2; }
pr=$1
shift
for f in "$@"; do
  [ -f "$f" ] || { echo "no such file: $f" >&2; exit 1; }
done
dir=$(mktemp -d)
trap 'git worktree remove --force "$dir" 2>/dev/null || true; rm -rf "$dir"' EXIT
for attempt in 1 2 3; do
  git fetch -q origin pr-images
  git worktree add -q --detach "$dir" origin/pr-images
  mkdir -p "$dir/$pr"
  cp "$@" "$dir/$pr/"
  git -C "$dir" add "$pr"
  git -C "$dir" diff --cached --quiet || git -C "$dir" commit -q -m "Images for #$pr"
  git -C "$dir" push -q origin HEAD:pr-images && break
  [ "$attempt" -lt 3 ] || { echo "pr-images refused the push 3 times" >&2; exit 1; }
  git worktree remove --force "$dir"
done
for f in "$@"; do
  echo "<img src=\"https://raw.githubusercontent.com/simopzz/dandori/pr-images/$pr/$(basename "$f")\" width=\"520\">"
done
