## Branching

Implement each issue on its own feature branch off `main`, named `<issue>-<slug>` (e.g. `6-profile`), created before the first commit. `main` takes work only through PRs. This overrides any skill step that says to commit to the current branch.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.
