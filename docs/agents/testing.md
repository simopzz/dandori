# Testing

Tests sit at two seams. Issues name them in their acceptance criteria.

## Seam 1: the app over HTTP

`internal/app/*_test.go`. `apptest.Start` boots the whole app in-process against its own freshly migrated Postgres + pgvector database, cloned per test; start the server once with `mise run testdb`. A test drives pages the way a browser does: it reads a form with `apptest.Form(t, page, id)`, posts it, follows a link with `apptest.Link(t, page, text)`, and checks what the next page shows through `text(body)` (the page's main content, without markup). Outbound ports are swapped with `apptest.WithPorts`: the scripted LLM (`agent/fakellm`) and the in-memory Esselunga (`esselunga/fakestore`), which can log its session out or take a product out of stock mid-Run. The store throttle is 1ms unless a test sets `app.Config.StoreThrottle`. the clock is `app.Config.Now`, set with `apptest.WithConfig` (see `startPlanning` in `planning_test.go`). Assert on what the user sees, and reach into the database with `h.Exec` only to stand in for something no page does yet, such as the planner storing a Plan Proposal. A form that swaps its own fragment (`httpx.IsHTMX`) is tested both ways: posted as htmx with `HTMXPostForm`, checking the fragment, and as a plain browser, checking the page it lands on.

## Seam 2: the domain core

`internal/domain/*_test.go`. Pure functions called directly, usually table-driven: Plan validation, Cycle lifecycle, nutrition. No database, no clock: the time a rule needs is passed in.

## Not in the suite

The Playwright store (`internal/esselunga/playwright`) is checked by hand against the real Esselunga site with `mise run smoke`, read-only, never in CI.

## Guardrails

`internal/web` holds tests that check the source instead of behaviour: colours are tokens, templates use the glossary's words, and htmx markup uses htmx 4's names with a no-JS fallback on every htmx form. `mise run check` runs everything CI runs.
