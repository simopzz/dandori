#!/usr/bin/env bash
# Whether the commits from <from> to <to> change what a deploy ships: the image's sources
# as .dockerignore lets them in (tests aside), the server's files in deploy/, or the CI
# workflow. Prints the changed files, and exits 0 if any of them ship, 1 if none do.
set -eu
[ $# -eq 2 ] || { echo "usage: scripts/ships.sh <from> <to>" >&2; exit 2; }
changed=$(git diff --name-only "$1" "$2")
printf '%s\n' "$changed"
# No pipefail: grep -q stops reading at the first match, which would fail grep -v.
grep -v '_test\.go$' <<<"$changed" |
  grep -Eq '^(go\.(mod|sum)|Dockerfile|\.dockerignore|cmd/|internal/|deploy/|\.github/workflows/ci\.yml$)'
