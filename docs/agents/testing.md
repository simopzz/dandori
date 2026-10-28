# Testing

Tests sit at two seams. Issues name them in their acceptance criteria. An adapter whose behaviour the fakes stand in for, and so no Seam-1 test reaches, is tested on its own against a fake of the service it calls.

## Seam 1: the app over HTTP

`internal/app/*_test.go`. `apptest.Start` boots the whole app in-process against its own freshly migrated Postgres + pgvector database, cloned per test; start the server once with `mise run testdb`. A test drives pages the way a browser does: it reads a form with `apptest.Form(t, page, id)`, posts it, follows a link with `apptest.Link(t, page, text)`, and checks what the next page shows through `text(body)` (the page's main content, without markup). Outbound ports are swapped with `apptest.WithPorts`: the scripted LLM (`agent/fakellm`), the in-memory Esselunga (`esselunga/fakestore`), which can log its session out or take a product out of stock mid-Run, and the bag-of-words embedder (`embedding/fakeembed`), so texts sharing more words rank closer, which can fail embedding Recipes or queries. A Run that tries one of the app's own agent tools is registered with `apptest.WithAppJob`. The store throttle is 1ms unless a test sets `app.Config.StoreThrottle`. the clock is `app.Config.Now`, set with `apptest.WithConfig` (see `startPlanning` in `planning_test.go`). Assert on what the user sees, and reach into the database with `h.Exec` only to stand in for something no page does yet, such as the planner storing a Plan Proposal. A form that swaps its own fragment (`httpx.IsHTMX`) is tested both ways: posted as htmx with `HTMXPostForm`, checking the fragment, and as a plain browser, checking the page it lands on.

## Seam 2: the domain core

`internal/domain/*_test.go`. Pure functions called directly, usually table-driven: Plan validation, Cycle lifecycle, nutrition. No database, no clock: the time a rule needs is passed in.

## Adapters

`internal/embedding/voyage/voyage_test.go`. The Voyage adapter against a local fake of Voyage's embeddings endpoint (`httptest`), which enforces Voyage's request limits: it checks the adapter splits what it embeds into requests within them and puts the vectors back in order.

## Not in the suite

The Playwright store (`internal/esselunga/playwright`) is checked by hand against the real Esselunga site with `mise run smoke`, read-only, never in CI.

## Guardrails

`internal/web` holds tests that check the source instead of behaviour: colours are tokens, templates use the glossary's words, and htmx markup uses htmx 4's names with a no-JS fallback on every htmx form. `mise run check` runs everything CI runs.
