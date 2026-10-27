# Testing

Tests sit at two seams. Issues name them in their acceptance criteria.

## Seam 1: the app over HTTP

`internal/app/*_test.go`. `apptest.Start` boots the whole app in-process against its own freshly migrated Postgres + pgvector database, cloned per test; start the server once with `mise run testdb`. A test drives pages the way a browser does: it reads a form with `apptest.Form(t, page, id)`, posts it, and checks what the next page shows through `text(body)` (the page's main content, without markup). Outbound ports are swapped with `apptest.WithPorts`; the clock is `app.Config.Now`, set with `apptest.WithConfig` (see `startPlanning` in `planning_test.go`). Assert on what the user sees, and reach into the database with `h.Exec` only to stand in for something no page does yet, such as the planner storing a Plan Proposal.

## Seam 2: the domain core

`internal/domain/*_test.go`. Pure functions called directly, usually table-driven: Plan validation, Cycle lifecycle, nutrition. No database, no clock: the time a rule needs is passed in.

## Guardrails

`internal/web` holds tests that check the source instead of behaviour: colours are tokens, and templates use the glossary's words. `mise run check` runs everything CI runs.
