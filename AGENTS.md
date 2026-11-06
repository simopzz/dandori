## Working on an issue

Implement each issue on its own feature branch off `v2`, named `<issue>-<slug>` (e.g. `6-profile`), created before the first commit. `v2` takes work only through PRs. This overrides any skill step that says to commit to the current branch.

`main` is v1, frozen: read it to port code whose model hasn't changed, never commit to it. See `docs/adr/0014-v2-is-a-new-codebase.md`.

Before writing code, list the domain decisions the issue leaves open (behaviour it implies but doesn't state) and confirm them with the user.

Before every commit, run `mise run check`: everything CI runs.

## Agent skills

### Issue tracker

Issues are tracked in GitHub Issues via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `GLOSSARY.md` + `docs/adr/` at the repo root. See `docs/agents/domain.md`.

### Code layout

Code is grouped by feature, each with handlers, a `Service` and a store. See `docs/adr/0006-package-by-feature-with-a-service-per-feature.md` before adding or moving code.

### Tests

Tests sit at the two seams issues name, Seam-1 (the app over HTTP) and Seam-2 (the domain core). See `docs/agents/testing.md` before writing one.

### UI

Any UI work follows the design system in `docs/design.md`: build pages from the shared templ components in `internal/web` and style with the tokens in `app.css`. The rules code review checks are in `docs/agents/coding-standards.md`. Load the `htmx-guidance` skill before writing htmx markup: the vendored htmx is version 4.

### Browser

Diagnosing a page in a browser, whether a style that won't apply, a slow load or swap, an accessibility failure or a leak, goes through the `chrome-devtools` MCP server in `.mcp.json`. Clicking through and screenshots go through `playwright-brave`. See `docs/agents/browser-tools.md` before opening either.

### PR screenshots

A PR body shows images through an orphan branch `pr-assets/<pr>`, holding them under `<pr>/`, referenced as `<img src="https://raw.githubusercontent.com/simopzz/dandori/pr-assets/<pr>/<pr>/name.png" width="520">`.

### Grafana

What the server did, in production or a `mise run dev` run (a slow request, an error, a beat), and dashboards of it, go through the `grafana` MCP server in `.mcp.json`: Grafana Cloud, where its spans and logs land. See `docs/agents/grafana.md` before querying.
