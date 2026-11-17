# Onboarding to Dandori

*Generated from the Understand-Anything knowledge graph at commit `ef39284`.*

## Project overview

**Dandori** (段取り) is a meal-planning co-pilot for a few invited Users, each cooking for themselves. It plans batch-cooked meals ("cook once, eat three times"), tracks what is in the kitchen, adapts when the schedule changes, and fills an online supermarket cart without ever checking out. The agent proposes and you approve, and every agent Run has a visible Trace of its steps.

| | |
|---|---|
| Languages | Go, templ, SQL, CSS, JavaScript, Markdown, YAML, TOML, shell |
| Frameworks | templ + htmx 4, pgx, goose, OpenTelemetry, Playwright, Docker, GitHub Actions |
| Data | PostgreSQL with pgvector on Supabase, with Supabase Auth for magic-link sign-in |
| Hosting | A single static binary on a Hetzner VPS, behind Caddy, run by Docker Compose (ADR-0017) |
| Model | Claude through the Anthropic Go SDK, set by `ANTHROPIC_API_KEY` and `DANDORI_MODEL`. `mise run dev` uses the demo LLM unless `DANDORI_DEMO_LLM=false` |
| Run it | `cp .env.example .env && mise run dev` serves http://127.0.0.1:8080 against a local Docker Postgres |

This is **v2**, a new codebase (ADR-0014). `main` is v1 and is frozen. Built so far:
- sign-in, Profile and Language
- Today
- Ingredients
- the Kitchen Ledger (stock and Portions)
- the Recipe Library
- pasting a recipe, which starts an agent Run that produces a Recipe Draft for you to check, then accept or discard
- Runs you can watch live, cancel and list

Read **`GLOSSARY.md`** first. The code uses its words exactly, and a guardrail test fails on the words it lists under *Avoid*.

## Architecture layers

Every import between layers points down this list. Nothing lower imports anything higher:

```
Entry & App Wiring      cmd/dandori, internal/app/app.go
        ↓
Features                auth · ingredients · kitchen · profile · recipes · runs · today
        ↓
Kitchen Ledger · Agent Engine · Web UI
        ↓
Domain Core · Platform Services (clock, db, httpx, telemetry)
```

Alongside the code there are four more layers: the database schema, the test harness, deployment and CI, and the docs and decisions.

| Layer | What it holds | Start with |
|---|---|---|
| **Entry & App Wiring** | `main.go` migrates, wires and serves. `app.go` builds every Service and mounts every route behind the sign-in, language-choice and ledger catch-up middlewares. | `internal/app/app.go` |
| **Features** | One package per feature, split into tier files named by prefix: `web_handlers.go`, `web_messages.go`, `web_form.go`, `web_views.templ`, `service*.go`, `store*.go`. | `internal/kitchen/` |
| **Kitchen Ledger** | Writes Movements, counts items to nothing, and brings each User's kitchen up to now on each request and on the scheduler beat (ADR-0019). | `internal/ledger/service.go` |
| **Agent Engine** | A hand-rolled LLM tool-use loop with Brief, Trace and retries (ADR-0003) behind an `LLM` port. `anthropic/` is the real Claude client, and `fakellm` is the scripted double for tests. | `internal/agent/loop.go` |
| **Web UI** | Shared templ components and layout, bilingual Messages, `Answer` (the one response every write gives), Notices, icons, and `app.css` tokens. | `internal/web/components.templ` |
| **Domain Core** | Pure rules with no I/O: Ingredients and fuzzy matching, Language, ledger arithmetic, Portions and Lots, Recipes, Slots. | `internal/domain/ledger.go` |
| **Platform Services** | The clock (the only source of time), the pgx pool and the run-as-User transaction, HTTP helpers, OpenTelemetry setup. | `internal/clock/clock.go` |
| **Database Schema** | Goose migrations 00001–00015: user, profile, kitchen, ingredient, movement, portion, recipe, run, trace_event, recipe_draft (00015 grants DELETE on it). | `00002_users.sql`, `00007_movement.sql` |
| **Seam-1 Tests & Harness** | The whole app over HTTP and in a browser, using the `apptest` harness: manual clock, fake Supabase Auth, fake LLM, and a Scenario DSL. | `internal/apptest/scenario.go` |
| **Deployment, CI & Config** | Dockerfile, compose, Caddyfile, the `ci.yml` check/build/deploy pipeline, `mise.toml` tasks, the pre-commit hook, and `provision.sh`. | `mise.toml` |
| **Docs & Decisions** | README, vision, GLOSSARY, 19 ADRs, the agent guides in `docs/agents/`, and the design system in `docs/design.md`. | `GLOSSARY.md` |

## Key concepts

- **The kitchen is an append-only ledger** (ADR-0013). Stock is never stored. It is the sum of Movements (counted, moved, staple, added), and Quantities are kept in tenths. A Count records what you *see*, and the ledger writes the difference. "Bringing the kitchen up to now" is idempotent and runs on every request (the `UpToNow` middleware) and on the scheduler beat.
- **Time is an input** (ADR-0014). Code reads the time only from the injected `clock.Clock`. A guardrail test fails on any `time.Now`, `time.Since` or `time.Until` outside `internal/clock`. Tests move a Manual clock and never sleep.
- **Users share nothing** (ADR-0015). Every owned table has row-level security keyed on `current_user_id()` from the Supabase JWT. Stores run inside `db.AsUser` transactions, so isolation is enforced by Postgres, not by `WHERE` clauses.
- **Package by feature, tiers by prefix** (ADR-0006). Shared packages never import features, and `app.go` holds the one route table. `internal/web/tiers_test.go` checks that each file belongs to one tier by its prefix and that only `store*` files touch the database. `.golangci.yml` depguard rules back this up.
- **Two languages, no text in templates.** Every visible string is a `web.Message{En, It}` in a `web_messages.go`, and a test checks that both translations exist with matching format verbs. A User's Language can change only until the first Food Term is written.
- **Boosted pages, one write answer** (ADR-0009). `hx-boost` sits on `<body>`, so handlers don't branch on `HX-Request`. A write returns a `web.Answer`: an htmx fragment with a floating Notice for htmx, or a redirect with a Notice for a plain browser. Fragments are used only where a row in a long list has to keep its place. Every form works without JavaScript.
- **Agent Runs are visible, live and fakeable.** A Run is one agent job, started in the background as its User. Each step becomes a Trace Event stored as kind + jsonb. The Run page streams new events over server-sent events (htmx SSE) until the Run ends, and you can cancel a Run or list them all. A Brief says what each kind of Run tells the model. `internal/agent/anthropic` talks to Claude in production, with prompt caching, a refusal fallback and retryable-error classification. `fakellm` scripts the replies in tests, and `DemoLLM` answers under `mise run dev`.
- **The agent proposes, you approve.** A parse Run never writes a Recipe directly. It leaves a Recipe Draft, which you check in the same Recipe form you edit with, then accept into the Library (creating any new Ingredients) or discard.
- **Guardrail tests encode the standards.** `internal/web/*_test.go` checks glossary words, colour literals (only in `app.css` `:root` tokens), htmx 4 attribute names, tiers and comment wrapping, and `internal/clock` checks time reads. `.githooks/pre-commit` runs `mise run guardrails`, and `mise run check` runs everything CI does.

## Guided tour

1. **What Dandori is.** `README.md`, `vision.md`, `GLOSSARY.md`: the product ideas and the vocabulary everything else uses.
2. **Entry point and wiring.** `cmd/dandori/main.go`, `internal/app/app.go` and `.env.example`. `main.go` chooses the real Anthropic client or the demo LLM. `app.go` has the most outgoing links in the graph. It builds every Service and mounts every route.
3. **Package by feature.** ADR-0006 and `internal/web/tiers_test.go`: the layout convention and the guardrail that enforces it.
4. **One feature end to end.** The Kitchen: `web_handlers.go` → `service.go` (each write in one transaction) → `web_views.templ`. Every other feature has the same shape.
5. **The Kitchen Ledger.** ADR-0013 and ADR-0019, then `internal/ledger/service.go` and `store.go`: Movements, the per-User kitchen lock, bring-up-to-now.
6. **The pure domain core.** `internal/domain/ledger.go` (replays Movements into Stock and Portions at any moment), `portion.go` (Lots and their capacity), `ingredient.go`, `slot.go`.
7. **Schema and row-level security.** `internal/db/db.go`, then migrations `00002_users` (`current_user_id()`), `00004_kitchen` and `00007_movement`, and ADR-0015.
8. **The Recipe Library.** `internal/recipes/service.go` and `store.go` (which also holds the Recipe Draft SQL), `internal/domain/recipe.go` (per-Portion nutrition as a floor), and the recipe table from migration 00012.
9. **The agent and Claude.** ADR-0003, then `internal/agent/agent.go` (the `LLM` port), `loop.go` (tool use, bounded turns, retries with backoff, Trace Events) and `anthropic/anthropic.go` (the Claude adapter `main.go` wires in when `ANTHROPIC_API_KEY` is set).
10. **Runs and live Traces.** `internal/runs/service_engine.go` (start, record, follow, cancel, list), `store.go` and migration 00014, then `web_handlers.go` and `web_views.templ`: the Runs list and a Run page whose Trace streams over SSE.
11. **Recipe Drafts: check and accept.** `recipes/service_parse.go` (the parse Run's brief and tool schema) → `service_draft.go` (accept into the Library or discard) → `web_handlers.go`, plus migration 00015.
12. **Web shell and design.** `docs/design.md`, ADR-0002, `layout.templ` (the tab bar, htmx, service worker), `components.templ` and `app.css`.
13. **Platform: clock and telemetry.** `internal/clock/clock.go` (Real and Manual, with beats) and `internal/telemetry/telemetry.go` (OTLP to Grafana Cloud, trace IDs on log lines).
14. **The two test seams.** `docs/agents/testing.md`, then `apptest.go` (an in-process app on a cloned migrated database), `scenario.go` (the Given/When/Then DSL), `fakellm` (scripted replies, failures, overloads and hangs) and `drafts_test.go` (a Seam-1 test that walks a Recipe Draft through check, accept and discard).
15. **Build, CI and deploy.** `mise.toml`, `.githooks/pre-commit`, `.github/workflows/ci.yml` (check → build image → SSH deploy → `/healthz` over TLS), the `Dockerfile` (distroless, one static binary) and `deploy/compose.yaml`.

## File map

### Entry & wiring
- `cmd/dandori/main.go`: `serve` migrates, wires telemetry, clock, auth and Runs, picks the Anthropic client or the demo LLM, then serves HTTP with graceful shutdown. `healthcheck` is the container probe.
- `cmd/dandori/main_test.go`: a guard that the production binary never links `internal/apptest`.
- `internal/app/app.go`: the composition root and route table.

### Features (`internal/<feature>/`)
| Feature | Service / store | Web |
|---|---|---|
| **auth** | `service.go` (magic links, sessions, refresh), `store_supabase.go` (Supabase Auth API, JWT checked against a cached JWKS), `store.go` (upserts the User row) | `web_handlers.go` (the `SignedIn` middleware, sign-in pages, secure session cookies) |
| **ingredients** | `service.go` (add, edit, delete, search and suggest; refuses taken or near-miss names and unit changes once counted), `store.go` | `web_handlers.go`, `web_form.go` (nutrition fields), `web_views.templ` |
| **kitchen** | `service.go` (Stock from the ledger; Counts, moves and portion additions, each in one transaction) | `web_handlers.go` (incl. the `UpToNow` middleware), `web_form.go`, `web_views.templ` |
| **profile** | `service.go` (Language and Slot times; the Language lock), `store.go` | `web_handlers.go` (the `LanguageChosen` middleware, first-run choice) |
| **recipes** | `service.go` (the Library), `service_parse.go` (the parse Run's brief and tool schema), `service_draft.go` (starts the Run, stores, accepts or discards the Draft), `service_demo_llm.go`, `store.go` (Recipes and Drafts) | `web_handlers.go` (incl. check, accept, discard), `web_form.go` (dynamic ingredient and step rows), `web_views.templ`, `static/recipe-form.js` |
| **runs** | `service_engine.go` (starts Runs in the background as their User, recovers panics, records each Trace Event, streams them to followers, cancels and lists), `store.go` | `web_handlers.go` (list, page, cancel, the SSE stream), `web_views.templ` (live Trace, cancel button, result link) |
| **today** | `service.go` (the Slot now or next) | `web_handlers.go`, `web_views.templ` |

### Below the features
- `internal/ledger/service.go`, `store.go`: Movements, per-User locks, beats and up-to timestamps.
- `internal/agent/agent.go`: the provider-neutral `LLM` interface and its wire types. `loop.go`: the loop. `brief.go`: the Brief and Tool types. `trace.go`: the Trace Event kinds and Failure reasons. `anthropic/`: the Claude client over the Anthropic Go SDK. `fakellm/`: scripted replies.
- `internal/domain/`: `ledger.go`, `portion.go`, `ingredient.go`, `ingredient_match.go` (one-slip and trigram fuzzy matching), `recipe.go`, `language.go`, `slot.go` (DST-aware).
- `internal/web/`: `components.templ`, `layout.templ`, `answer.go`, `notice.go`, `i18n.go`, `messages.go`, `icons.go` and `pixels.go` (pixel art as SVG), `markdown.go` (agent Markdown with raw HTML stripped), `static/app.css`, and the PWA files `root/manifest.webmanifest` and `root/sw.js`.
- `internal/db/db.go` (pool, embedded migrations, `Querier`) and `user.go` (`AsUser`); `internal/clock/clock.go`; `internal/httpx/httpx.go`; `internal/telemetry/telemetry.go`.

### Tests
- `internal/app/*_test.go`: Seam 1, one file per area (count, move, movements, portions, recipes, parse, drafts, runs, ingredients, signin, language, today, spans, browser, busy, sheets, notice, shell, startup).
- `internal/apptest/`: `apptest.go`, `browser.go` (Playwright), `fakeauth.go` (fake Supabase Auth), `scenario.go`, and the form, link, signin and text helpers.
- `internal/domain/*_test.go`: Seam 2, including `ingredient_stress_test.go`, which runs the matcher on real kitchen names with generated typos.

### Ops and config
- `mise.toml` (tool versions plus the `dev`, `generate`, `check`, `guardrails`, `test`, `devdb`, `browsers` and `db:schema` tasks), `.env.example` (including `ANTHROPIC_API_KEY`, `DANDORI_DEMO_LLM`, `DANDORI_MODEL`), `.golangci.yml`, `.mcp.json`, `go.mod`.
- `Dockerfile`, `.dockerignore`, `deploy/compose.yaml`, `deploy/Caddyfile`, `.github/workflows/ci.yml`, `.githooks/pre-commit`, `scripts/provision.sh` (an interactive production provisioning wizard).

## Complexity hotspots

These files were rated *complex*. Read the code around them before changing them.

| Area | Files | Why to be careful |
|---|---|---|
| Wiring | `internal/app/app.go` | Every Service and route meets here. Middleware order (sign-in → language → up-to-now) matters. |
| Ledger arithmetic | `internal/domain/ledger.go` | Replay over time, Quantities in tenths, Portion Movements. Small changes ripple into every stock figure. Pair them with `ledger_test.go` and `portion_test.go`. |
| Kitchen | `kitchen/service.go`, `web_handlers.go`, `web_views.templ` | One transaction per write, stale-form refusals, Portion Lots, and htmx fragments that keep your place. |
| Ingredients | `ingredients/service.go`, `web_handlers.go`, `web_views.templ`, `domain/ingredient_match.go` | Near-miss suggestions, the unit lock once counted, deleting while Movements reference the Ingredient. |
| Recipes and Drafts | `recipes/service_parse.go`, `store.go`, `web_form.go`, `web_handlers.go`, `web_views.templ` | The LLM tool schema and conversion into domain values, dynamic form rows, and draft locking on accept. |
| Runs | `runs/service_engine.go`, `web_views.templ` | Background goroutines per Run, live followers, and cancellation, all with SSE in the browser. |
| Claude client | `agent/anthropic/anthropic.go` | Mapping to and from the SDK, prompt caching, the refusal fallback, and which failures count as retryable. |
| Auth | `auth/store_supabase.go` | JWT verification against a cached JWKS (RSA and ECDSA), session refresh and revocation. This is security code. |
| Shared UI | `web/components.templ`, `static/app.css`, `docs/design.md` | Every page depends on them, and guardrails cover colours, htmx names and text. |
| Test harness | `apptest/apptest.go`, `browser.go`, `fakeauth.go`, `scenario.go` | Every Seam-1 test runs on it, so a change here affects the whole suite. |
| Ops | `scripts/provision.sh` | Provisions production: DNS, server, Supabase, secrets. |
| Vocabulary | `GLOSSARY.md` | It's a contract: `glossary_test.go` fails on avoided words in page text and Go identifiers. |

## Before your first change

1. Read `AGENTS.md` and `docs/agents/coding-standards.md`, `testing.md` and `domain.md`.
2. Work on a branch `<issue>-<slug>` off `v2`. `v2` only takes PRs.
3. Write the test at the right seam: Seam 1 for behaviour over HTTP, Seam 2 for a domain rule.
4. Run `mise run check` before every commit. The pre-commit hook runs only the fast guardrails.
