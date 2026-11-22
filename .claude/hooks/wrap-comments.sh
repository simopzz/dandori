#!/usr/bin/env bash
# PostToolUse hook on Edit|Write: rewraps the Go comments of the .go or .templ file just
# written, so TestCommentsWrap never fails a check. A comment it can't rewrap (a list)
# goes back to the agent, on stderr with exit 2, to rewrap by hand.
f=$(jq -r '.tool_input.file_path // empty')
case "$f" in
  "$CLAUDE_PROJECT_DIR"/*.go | "$CLAUDE_PROJECT_DIR"/*.templ) ;;
  *) exit 0 ;;
esac
cd "$CLAUDE_PROJECT_DIR" || exit 0
out=$(MISE_ACTIVATE_AGGRESSIVE=1 mise exec -- go run ./scripts/wrap "$f" 2>&1) || { echo "$out" >&2; exit 2; }
case "$out" in
  *"by hand"*) echo "$out" >&2; exit 2 ;;
esac
