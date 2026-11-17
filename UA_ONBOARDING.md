# Onboarding to Dandori

*Generated from the Understand-Anything knowledge graph at commit `28daec6`.*

## Project overview

**Dandori** (段取り) is a meal-planning co-pilot for a few invited Users, each cooking for themselves. It plans batch-cooked meals ("cook once, eat three times") one grocery Delivery (a Cycle) at a time. It also tracks what is in the kitchen, adapts when the schedule changes, and fills an online supermarket cart without ever checking out. The agent proposes and you approve, and every agent Run has a visible Trace of its steps.

| | |
|---|---|
| Languages | Go, templ, SQL, CSS, JavaScript, Markdown, YAML, TOML, shell |
| Frameworks | templ + htmx 4, pgx, goose, OpenTelemetry, Playwright, Docker, GitHub Actions |
| Data | PostgreSQL with pgvector on Supabase, with Supabase Auth for magic-link sign-in |
| Hosting | A single static binary on a Hetzner VPS, behind Caddy, run by Docker Compose (ADR-0017) |
| Model | Claude through the Anthropic Go SDK, set by `ANTHROPIC_API_KEY` and `DANDORI_MODEL`. `mise run dev` uses the demo LLM unless `DANDORI_DEMO_LLM=false` |
| Observability | Spans and logs over OTLP to Grafana Cloud, and Grafana Faro in the browser |
| Run it | `cp .env.example .env && mise run dev` serves http://127.0.0.1:8080 against a local Docker Postgres |

This is **v2**, a new codebase (ADR-0014). `main` is v1 and is frozen. Built so far:
- sign-in, and the first-run Language choice
- the Profile:
  - Slot times
  - nutrition Targets
  - the Meal Out estimate
  - food exclusions and allergies
  - appliances
  - the Planning Brief
- Today
- Ingredients
- the Kitchen Ledger (stock and Portions)
- the Recipe Library
- pasting a recipe, which starts an agent Run that produces a Recipe Draft for you to check, then accept or discard
- Runs you can watch live, cancel and list
- Cycles: open one with its Delivery, drop an upcoming one, run a Kitchen Check, and plan it into a pending Plan Proposal

Read **`GLOSSARY.md`** first. The code uses its words exactly, and a guardrail test fails on the words it lists under *Avoid*.

## Architecture layers

Every import between layers points down this list. Nothing lower imports anything higher:

```
Entry & App Wiring      cmd/dandori, internal/app/app.go
        ↓
Today                   the top feature: nothing but app reads it
        ↓
Planning                Cycles and Plan Proposals: only Today and app read it
        ↓
Features                auth · ingredients · kitchen · profile · recipes · runs
        ↓
Kitchen Ledger · Agent Engine · Web UI
        ↓
Domain Core · Platform Services (clock, db, httpx, telemetry)
```

`.golangci.yml` enforces the order with depguard rules (`planning-is-downstream`, `today-is-the-top`), and `internal/web/tiers_test.go` enforces the file tiers inside each feature. Alongside the code there are four more layers: the database schema, the test harness, deployment and CI, and the docs and decisions.

| Layer | What it holds | Start with |
|---|---|---|
| **Entry & App Wiring** | `main.go` reads the environment, wires telemetry, the LLM and Supabase auth, and serves. `app.go` migrates the database, builds every Service and mounts every route behind the sign-in, language-choice and kitchen catch-up middlewares. | `internal/app/app.go` |
| **Features** | One package per feature, split into tier files named by prefix: `web_handlers.go`, `web_messages.go`, `web_form.go`, `web_views.templ`, `service*.go`, `store*.go`. | `internal/kitchen/` |
| **Kitchen Ledger** | Writes Movements under the kitchen lock, counts Staples to nothing, and brings each User's kitchen up to now on each request and on the scheduler beat (ADR-0019). | `internal/ledger/service.go` |
| **Agent Engine** | A hand-rolled LLM tool-use loop with Brief, Trace and retries (ADR-0003) behind an `LLM` port. `anthropic/` is the real Claude client, and `fakellm` is the scripted double for tests. | `internal/agent/loop.go` |
| **Web UI** | Shared templ components and layout, bilingual Messages, `Answer` (the one response every write gives), Notices, pixel icons and art, `app.css` tokens, Faro, and the PWA files. | `internal/web/components.templ` |
| **Domain Core** | Pure rules with no I/O: appliances, Cycles, dates and times, foods, Ingredients and fuzzy matching, Language, ledger arithmetic, Meal Out, Plans, Portions, Profile, Recipes, Slots, Targets. | `internal/domain/doc.go` |
| **Platform Services** | The clock (the only source of time), the pgx pool and the run-as-User transaction, HTTP helpers, OpenTelemetry setup. | `internal/clock/clock.go` |
| **Database Schema** | Goose migrations 00001–00022, covering users, profile, kitchen, ingredient, movement, portion, recipe, run, trace_event, recipe_draft, cycle and the planning tables (cycle_run, plan_proposal, prep_session, planned_portion, kept_portion). | `00002_users.sql`, `00007_movement.sql`, `00022_plan_proposal.sql` |
| **Seam-1 Tests & Harness** | The whole app over HTTP and in a browser, using the `apptest` harness: a Manual clock, fake Supabase Auth, the fake LLM, and a Scenario DSL. | `internal/apptest/scenario.go` |
| **Deployment, CI & Config** | Dockerfile, compose, Caddyfile, the `ci.yml` check/build/deploy pipeline, `mise.toml` tasks, the pre-commit hook, and `provision.sh`. | `mise.toml` |
| **Docs & Decisions** | README, vision, GLOSSARY, 21 ADRs, the agent guides in `docs/agents/`, and the design system in `docs/design.md`. | `GLOSSARY.md` |

## Key concepts

- **The kitchen is an append-only ledger** (ADR-0013). Stock is never stored. It is the sum of Movements (counted, moved, staple, added), and Quantities are kept in tenths. A Count records what you *see*, and the ledger writes the difference. "Bringing the kitchen up to now" is idempotent and runs on every request (the `UpToNow` middleware) and on the scheduler beat.
- **Time is an input** (ADR-0014). Code reads the time only from the injected `clock.Clock`. A guardrail test fails on any `time.Now`, `time.Since` or `time.Until` outside `internal/clock`. Tests move a Manual clock and never sleep.
- **Users share nothing** (ADR-0015). Every owned table has row-level security keyed on `current_user_id()` from the Supabase JWT. Stores run inside `db.AsUser` transactions, so Postgres enforces the isolation, not `WHERE` clauses.
- **Package by feature, tiers by prefix** (ADR-0006). Shared packages never import features, and `app.go` holds the one route table. `internal/web/tiers_test.go` checks that each file belongs to one tier by its prefix and that only `store*` files touch the database.
- **Two languages, no text in templates.** Every visible string is a `web.Message{En, It}` in a `web_messages.go`, and a test checks that both translations exist with matching format verbs. Dates and times are written each Language's way. A User's Language can change only until the first Food Term is written.
- **Boosted pages, one write answer** (ADR-0009). `hx-boost` sits on `<body>`, so handlers don't branch on `HX-Request`. A write returns a `web.Answer`: an htmx fragment with a floating Notice for htmx, or a redirect with a Notice for a plain browser. Fragments are used only where a row in a long list has to keep its place. Every form works without JavaScript.
- **Agent Runs are visible, live and fakeable.** A Run is one agent job, started in the background as its User. Each step becomes a Trace Event stored as kind + jsonb. The Run page streams new events over server-sent events until the Run ends, and you can cancel a Run or list them all. A Brief says what each kind of Run tells the model. `internal/agent/anthropic` talks to Claude in production, with prompt caching, a refusal fallback and retryable-error classification. `fakellm` scripts the replies in tests, and `DemoLLM` answers under `mise run dev`.
- **The agent proposes, you approve.** A parse Run never writes a Recipe. It leaves a Recipe Draft, which you check in the same form you edit Recipes with, then accept or discard. A planning Run never applies a Plan. It leaves a pending Plan Proposal, and there is at most one pending per User (ADR-0008).
- **Cycles and planning.** A Cycle is the stretch one Delivery must last. Its status (upcoming, current, closed) is derived from the clock, never stored, and it becomes current when its Delivery window ends. The planner's prompt is built from the Profile, the Recipe Library and the kitchen contents. The model drafts with `check_plan` and finishes with `submit_plan`, and both validate the draft against the domain's hard rules (a Slot filled twice or left empty, a Portion eaten before it is prepared, an unknown Portion, a Slot outside the Cycle). Nutrition Targets are soft averages, and only hard rules stop a Proposal (ADR-0018).
- **Appliances are a fixed list** (ADR-0020). Recipes and the Profile match appliance kinds exactly, and each kind carries a free-text model note.
- **Guardrail tests encode the standards.** `internal/web/*_test.go` checks glossary words, colour literals (only in `app.css` `:root` tokens), htmx 4 attribute names, tiers, forms, type and comment wrapping, and `internal/clock` checks time reads. `.githooks/pre-commit` runs `mise run guardrails`, and `mise run check` runs everything CI does.

## Guided tour

1. **What Dandori is.** `README.md`, `vision.md` and `GLOSSARY.md`: the product ideas, a Cycle walked through week by week, the M1/M2 roadmap, and the vocabulary everything else uses.
2. **The single binary entry.** `cmd/dandori/main.go` reads `DANDORI_*` and the Anthropic settings, wires telemetry, picks the real Claude client or the demo LLM, connects Supabase auth and serves. Read it for a map of every runtime dependency.
3. **The composition root and tiers.** `internal/app/app.go` (the most outgoing links in the graph), ADR-0006, and `.golangci.yml`'s depguard rules.
4. **The pure domain core.** `internal/domain/doc.go` states the contract: callers load state, ask the domain, and persist the answer. Then read `cycle.go`, `slot.go`, `portion.go` and `plan.go`.
5. **Database and row security.** `internal/db/db.go` (traced pool, embedded migrations, `Querier`), `user.go` (`AsUser`), and migration `00002_users` (`current_user_id()` and the `own_rows` policy).
6. **The Kitchen Ledger.** ADR-0013 and ADR-0019, `internal/domain/ledger.go` for the arithmetic, then `internal/ledger/service.go` and `store.go` for the write gateway and bring-up-to-now.
7. **Anatomy of a feature.** The kitchen: `web_handlers.go` → `service.go` (Stock Items and Portion Lots from the Ledger, each write in one transaction under the kitchen lock), with `internal/web/tiers_test.go` keeping every feature in shape.
8. **Web shell and design.** `internal/web/answer.go` (ADR-0009), `i18n.go` (the User's Language through the request), `docs/design.md` and `static/app.css` (the Pixel ledger tokens and components).
9. **The hand-rolled agent.** ADR-0003, then `internal/agent/agent.go` (the `LLM` port), `brief.go`, `loop.go` (tool use, retries with backoff, Trace Events) and `anthropic/anthropic.go`.
10. **Runs and live Traces.** `internal/runs/service_engine.go` (start, record, follow, cancel, fail Runs a crash left going), `web_handlers.go` (the SSE stream), `agent/trace.go`, and `recipes/service_parse.go`, the simplest Run kind.
11. **Planning a Cycle.** `internal/planning/service.go` (open and drop Cycles) and `service_plan.go` (the planning Run, its prompt and the `check_plan`/`submit_plan` tools), with migrations `00021_cycle` and `00022_plan_proposal`.
12. **Today and the clock.** `internal/today/service.go` composes the Profile's Slot times, the kitchen's last beat and the current Cycle into the Day you see. `internal/clock/clock.go` has the real Clock that beats on a ticker and the Manual clock tests move.
13. **Testing at two seams.** `docs/agents/testing.md`, `apptest.go` (an in-process app on a cloned database), `scenario.go` (the Given/When/Then DSL), `fakellm`, and `internal/app/planning_test.go`, which tests the planner end to end.
14. **Build, ship and observe.** `mise.toml`, the `Dockerfile` (distroless, one static binary), `deploy/compose.yaml`, `.github/workflows/ci.yml` (templ drift, the full suite, then deploy), and `internal/telemetry/telemetry.go` (OTLP to Grafana Cloud).

## File map

### Entry & wiring
- `cmd/dandori/main.go`: `serve` wires telemetry, the clock, auth and Runs, picks the Anthropic client or the demo LLM, then serves HTTP with graceful shutdown. `healthcheck` is the container probe.
- `cmd/dandori/main_test.go`: a guard that the production binary never links `internal/apptest`.
- `internal/app/app.go`: the composition root and route table.

### Features (`internal/<feature>/`)
| Feature | Service / store | Web |
|---|---|---|
| **auth** | `service.go` (magic links, sessions, refresh, sign-out), `store_supabase.go` (the Supabase Auth API, with JWTs checked against a cached JWKS), `store.go` (upserts the User row) | `web_handlers.go` (the `SignedIn` middleware, sign-in pages, secure session cookies) |
| **profile** | `service.go` (Language, Slot times, Targets, Meal Out, exclusions and allergies, appliances, Planning Brief; the Language lock), `store.go` (the profile row and the User's Food Terms) | `web_handlers.go` (the `LanguageChosen` middleware and the Profile page, saved section by section) |
| **ingredients** | `service.go` (add, edit, delete, search and suggest; refuses taken or near-miss names, unit changes once counted, and deleting an Ingredient still in use), `store.go` | `web_handlers.go` (incl. marking a Staple), `web_form.go` (nutrition fields), `web_views.templ` |
| **kitchen** | `service.go` (Stock Items and Portion Lots from the ledger; Counts, moves and Portions, each in one transaction) | `web_handlers.go` (incl. the `UpToNow` middleware), `web_form.go`, `web_views.templ` |
| **recipes** | `service.go` (the Library), `service_parse.go` (the parse Run's brief and `submit_recipe` schema), `service_draft.go` (starts the Run, stores, accepts or discards the Draft), `service_demo_llm.go`, `store.go` (Recipes and Drafts) | `web_handlers.go` (incl. check, accept, discard), `web_form.go` (dynamic ingredient and step rows), `web_views.templ` |
| **runs** | `service_engine.go` (Runs in the background as their User, Trace Events, followers, cancel, list), `service_spans.go` (a span per model call, never the prompts), `store.go` | `web_handlers.go` (list, page, cancel, the SSE stream), `web_views.templ` |
| **planning** | `service.go` (the current and upcoming Cycle, why one can't be opened, open and drop), `service_plan.go` (the planning Run and pending Plan Proposals), `store.go` (Cycles, the Profile lock that serialises opening, planning-Run bookkeeping, Proposals with their Prep Sessions) | `web_handlers.go` (the Cycle page, the Kitchen Check, starting a planning Run) |
| **today** | `service.go` (the Day: Slot times, the last beat, the current Cycle) | `web_handlers.go`, `web_messages.go` (set-up links before any Plan exists) |

### Below the features
- `internal/ledger/service.go`, `store.go`: Movements, Portions, per-User kitchen locks, beats and up-to timestamps.
- `internal/agent/`:
  - `agent.go`: the provider-neutral `LLM` port and its wire types
  - `loop.go`: the tool-use loop
  - `brief.go`: the Brief and Tool types
  - `trace.go`: the Trace Event kinds and Failure reasons
  - `anthropic/`: the Claude client
  - `fakellm/`: scripted replies for tests
- `internal/domain/`:
  - `cycle.go`: a Cycle, its Delivery window and derived status
  - `plan.go`: Prep Sessions, kept Portions and the hard rules
  - `ledger.go`
  - `portion.go`
  - `ingredient.go`, `ingredient_match.go`: one-slip and trigram fuzzy matching
  - `recipe.go`
  - `slot.go`: DST-aware, and `TodayAt`
  - `target.go`, `meal_out.go`, `foods.go`, `appliance.go`, `profile.go`, `language.go`, `datetime.go`
- `internal/web/`:
  - `components.templ`, `layout.templ`
  - `answer.go`, `notice.go`
  - `i18n.go`, `messages.go`
  - `icons.go`, `pixels.go`: pixel art as SVG
  - `markdown.go`: agent Markdown with raw HTML stripped
  - `static/app.css`
  - `static/faro.js`
  - the PWA files `root/manifest.webmanifest` and `root/sw.js`
- `internal/db/db.go` (pool, embedded migrations, `Querier`) and `user.go` (`AsUser`); `internal/clock/clock.go`; `internal/httpx/httpx.go`; `internal/telemetry/telemetry.go`.

### Tests
- `internal/app/*_test.go`: Seam 1, one file per area, including `cycle_test.go`, `planning_test.go`, `profile_test.go`, `appliances_test.go`, `targets_test.go`, `meal_out_test.go`, `drafts_test.go`, `runs_test.go`, `spans_test.go` and `browser_test.go`.
- `internal/apptest/`: `apptest.go`, `browser.go` (Playwright), `fakeauth.go` (fake Supabase Auth), `scenario.go`, and the form, link, signin and text helpers.
- `internal/domain/*_test.go`: Seam 2, including `plan_test.go`, `cycle_test.go` and `ingredient_stress_test.go`, which runs the matcher on real kitchen names with generated typos.

### Ops and config
- `mise.toml`: tool versions, plus the tasks:
  - `dev`, `generate`, `build`, `lint`
  - `test`, `check`, `guardrails`
  - `testdb`, `devdb`, `browsers`
  - `db:schema`
  - `graph:dashboard`, which serves this knowledge graph's dashboard from `.ua`
- `.env.example`, which includes `ANTHROPIC_API_KEY`, `DANDORI_DEMO_LLM` and `DANDORI_MODEL`
- `.golangci.yml`, `.mcp.json`, `go.mod`
- `Dockerfile`, `.dockerignore`, `deploy/compose.yaml`, `deploy/Caddyfile`
- `.github/workflows/ci.yml`, `.githooks/pre-commit`
- `scripts/provision.sh`, an interactive production provisioning wizard

## Complexity hotspots

These files were rated *complex*. Read the code around them before changing them.

| Area | Files | Why to be careful |
|---|---|---|
| Wiring | `internal/app/app.go` | Every Service and route meets here. Middleware order (sign-in → language → up-to-now) matters. |
| Ledger arithmetic | `internal/domain/ledger.go` | Replay over time, Quantities in tenths, Portion Movements. Small changes ripple into every stock figure, so pair them with `ledger_test.go` and `portion_test.go`. |
| Ingredient model | `internal/domain/ingredient.go` | Units, Nutrition, the shared Problem vocabulary and per-Language name normalisation, which every feature relies on. |
| Planning | `planning/service_plan.go`, `store.go` | The planner prompt, the tool schemas checked against hard rules, the Profile lock, and the one-pending-Proposal invariant. |
| Kitchen | `kitchen/service.go`, `web_handlers.go` | One transaction per write, stale-form refusals, Portion Lots, and htmx fragments that keep your place. |
| Ingredients | `ingredients/service.go`, `web_handlers.go` | Near-miss suggestions, the unit lock once counted, deleting while Movements or Recipes reference the Ingredient. |
| Recipes and Drafts | `recipes/service_parse.go`, `store.go`, `web_form.go`, `web_handlers.go` | The LLM tool schema and conversion into domain values, dynamic form rows, and draft locking on accept. |
| Profile | `profile/web_handlers.go` | Many independently saved sections and the first-sign-in Language middleware. |
| Runs | `runs/service_engine.go`, `web_handlers.go` | Background goroutines per Run, live followers and cancellation, all with SSE in the browser. |
| Claude client | `agent/anthropic/anthropic.go` | Mapping to and from the SDK, prompt caching, the refusal fallback, and which failures count as retryable. |
| Auth | `auth/store_supabase.go` | JWT verification against a cached JWKS (RSA and ECDSA), session refresh and revocation. This is security code. |
| Shared UI | `web/icons.go`, `static/app.css`, `docs/design.md` | Every page depends on them, and guardrails cover colours, htmx names and text. |
| Test harness | `apptest/apptest.go`, `browser.go`, `fakeauth.go`, `scenario.go` | Every Seam-1 test runs on it, so a change here affects the whole suite. |
| Ops | `scripts/provision.sh` | It provisions production: DNS, server, Supabase, secrets. |
| Vocabulary | `GLOSSARY.md` | It's a contract: `glossary_test.go` fails on avoided words in page text and Go identifiers. |

## Before your first change

1. Read `AGENTS.md`, then `docs/agents/coding-standards.md`, `testing.md` and `domain.md`.
2. Work on a branch `<issue>-<slug>` off `v2`. `v2` only takes PRs.
3. List the domain decisions the issue leaves open and confirm them before writing code.
4. Write the test at the right seam: Seam 1 for behaviour over HTTP, Seam 2 for a domain rule.
5. Run `mise run check` before every commit. The pre-commit hook runs only the fast guardrails.
