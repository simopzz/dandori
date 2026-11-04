# Testing

Tests sit at two seams. Issues name them in their acceptance criteria. An adapter whose behaviour the fakes stand in for, and so no Seam-1 test reaches, is tested on its own against a fake of the service it calls.

Time is an input (ADR-0014): the app reads it only from the `clock.Clock` it is built with, and `internal/clock`'s guardrail fails any `time.Now`, `time.Since` or `time.Until` elsewhere, tests and templates included. A test moves the clock; it never waits for it.

## Seam 1: the app over HTTP

`internal/app/*_test.go`. `apptest.Start` boots the whole app in-process against its own freshly migrated Postgres + pgvector database, cloned per test; start the server once with `mise run testdb`. The app runs on `h.Clock`, a `clock.Manual` standing at the start of Monday 2 November 2026 in `clock.DefaultZone` (Europe/Rome) until the test moves it. Assert on what the User sees, and reach into the database with `h.Exec` only to stand in for something no page does yet. Seam 1 has two layers.

### Scenarios (the bulk)

A scenario is a Go test told as Given/When/Then over the clock, through `apptest.NewScenario(t)`:

```go
s := apptest.NewScenario(t)
s.At("Tue 10:00")    // Given: the clock moves forward to the next Tuesday, 10:00
s.Open("/")          // When: the User opens a page
s.Sees("Tuesday 3 November") // Then: its main content shows this
s.NextDay()          // the same time tomorrow
```

Every page but sign-in needs a signed-in User. `apptest` starts a fake Supabase Auth beside the app (`h.Auth`): it emails magic links only to invited Users, into a mailbox the test reads, and signs sessions that expire an hour after they're issued by the app's clock. Moving the clock past that takes the app through a refresh, as in production. `s.Invite(email)` stands in for an invitation from Supabase's dashboard, and `s.SignsIn(email)` signs in through the pages and the emailed link (`h.Invite` and `h.SignIn` outside a scenario, `b.SignIn` in a browser). A first sign-in lands on the Language choice, before anything else: `s.ChoosesLanguage("English")` picks one, and `h.SignUp` and `b.SignUp` sign in for the first time and choose English, for a test that needs only a signed-in User. Each owned table's rows are seen only by their User through row-level security, so a fixture inserted with `h.Exec` (as the admin role) needs that User's `user_id`.

`At` takes a weekday and a time, the next one from now, or a date and a time (`"2026-11-03 10:00"`); the clock never moves back. Each step goes on a timeline of what happened at what time, which a failed scenario prints. A step the DSL lacks goes on `Scenario` as a method that records itself, rather than into one test; a scenario can still use the `Harness` it embeds. The lower-level helpers drive pages the way a browser does: `apptest.Form(t, page, id)` reads a form to post, `apptest.Link(t, page, text)` follows a link, and `apptest.Text(body)` is the page's main content without markup. A form that swaps its own fragment (`httpx.IsHTMX`) is tested both ways: posted as htmx with `HTMXPostForm`, checking the fragment, and as a plain browser, checking the page it lands on.

### Browser tests (a few per page)

`h.Browser(t)` opens a headless Chromium on the app through playwright-go, for each page's main path as the User gets it, with htmx, CSS and scripts: `Open`, `Sees`, and the rest of `playwright.Page`. It sets the clock with `SetClock`, which posts to `POST /_test/clock`: a switch only `apptest` serves, so the production binary, which `cmd/dandori`'s guardrail checks never links `apptest`, has none. Install the browser once with `mise run browsers`. Without it, browser tests skip locally and fail in CI, which installs it.

## Seam 2: the domain core

`internal/domain/*_test.go`, once v2 has a domain core. Pure functions called directly, usually table-driven. No database, no clock: the time a rule needs is passed in.

## Guardrails

Tests that check the source instead of behaviour. `internal/web`: colours are tokens, pages show only Messages, each with both translations, and use the glossary's words, and htmx markup uses htmx 4's names with a no-JS fallback on every htmx form. `internal/clock`: nothing else reads the machine's time. `cmd/dandori`: the binary has no clock switch. `mise run check` runs everything CI runs.
