# Onboarding to Dandori

*Generated from the Understand-Anything knowledge graph at commit `03fb41b` (the current HEAD of `v2`).*

## Project Overview

**Dandori** (段取り, the Japanese art of arranging a task so it runs smoothly) is a meal-planning co-pilot for a few invited Users, each one person cooking for themselves. It plans batch-cooked meals, tracks what is in the kitchen, adapts to schedule changes, and fills an online supermarket cart — never checking out.

The whole product hangs on one idea: **cook once, eat three times**. One dinner yields three Portions: one eaten now, one as lunch two days later, one for the freezer. Planning happens per **Cycle**, the stretch one supermarket **Delivery** has to last, so delivery costs stay down. The agent proposes, the User approves: a change to the Plan only takes effect once it is accepted, and every agent **Run** carries a live **Trace** of its steps.

| | |
|---|---|
| Languages | Go, templ, SQL, CSS, JavaScript, shell, YAML, TOML, JSON, Markdown, Dockerfile |
| Frameworks | templ + htmx 4, pgx, goose, OpenTelemetry (OTLP), Playwright, Docker, GitHub Actions |
| Database | PostgreSQL with pgvector on Supabase; Supabase Auth for magic-link sign-in; row-level security isolates every User |
| Model | Claude through the Anthropic Go SDK (`ANTHROPIC_API_KEY`, `DANDORI_MODEL`); `mise run dev` uses the demo LLM unless `DANDORI_DEMO_LLM=false` |
| Hosting | One static binary on a Hetzner VPS behind Caddy, deployed by GitHub Actions (ADR-0017) |
| Observability | OTLP spans and logs to Grafana Cloud; Grafana Faro in the browser |
| Run it | `cp .env.example .env && mise run dev` → http://127.0.0.1:8080 against a local Docker Postgres |
| Task runner | `mise` — `generate`, `build`, `test`, `testdb`, `browsers`, `check`, `guardrails`, `devdb`, `dev`, `wrap`, `lint`, `db:schema`, `pr-image` |

Two facts orient you before you read a line of code:

- **This is v2, a new codebase.** `main` is v1 and is frozen; code whose model did not change is ported, the rest is rewritten (ADR-0014). `v2` takes work only through PRs.
- **Time is an input, never `now()`.** The app reads the time only from the `clock.Clock` it is built with. A guardrail test fails any `time.Now`, `time.Since` or `time.Until` outside `internal/clock`, templates included. Tests move a manual clock; they never wait.

Read **`GLOSSARY.md`** first. The code uses its words exactly — Cycle, Delivery, Plan, Slot, Plan Proposal, Verdict, Portion, Movement, Stock Item, Kitchen Ledger, Grocery List, Run, Trace — and a guardrail test fails the build on the words the glossary lists under *Avoid*.

## Architecture Layers

The knowledge graph groups the 310 tracked files into ten layers. Import directions point down this list; `.golangci.yml` depguard rules and `internal/web/tiers_test.go` enforce it.

### Entry & Runtime Wiring

The dandori binary and internal/app/app.go compose the whole app, alongside the process-wide services they stand on: the clock, the shared HTTP helpers and the OpenTelemetry and slog setup.

- `cmd/dandori/main.go` — the whole program: reads configuration from the environment, serves the app, or answers the container's health check.
- `internal/app/app.go` — the wiring: opens the database, builds every feature's Service, maps every HTTP route. The highest fan-out in the graph.
- `internal/clock/clock.go` — the one place Dandori reads the time: a real clock and a manual one tests set and step, both able to beat on a schedule.
- `internal/httpx/httpx.go` — the small HTTP helpers every handler shares: path ids, htmx detection, rendering, recording refusals on the request's span.
- `internal/telemetry/telemetry.go` — the OTLP span and log exporters and the slog logger that stamps each line with its span ids and the signed-in User's id.

### Seam-1 Tests & Harness

The Seam-1 suite in internal/app drives the app over HTTP and in a browser on a real migrated Postgres, using the apptest harness and the dev-mode fake Auth, mailbox and time-travel routes under /_test/.

- `internal/apptest/apptest.go` — starts Dandori in-process over HTTP against its own freshly migrated Postgres: a manual clock the test steps, a fake Auth, a scripted model, the HTTP helpers.
- `internal/apptest/scenario.go` — the Given/When/Then DSL over the app's clock, keeping a timeline a failed test prints.
- `internal/apptest/browser.go` — drives a real headless Chromium for the tests that need the UI as the User gets it.
- `internal/apptest/form.go` — reads a form to post, the way a browser would.
- `internal/devmode/devmode.go` — mounts `/_test/clock`, `/_test/seed`, `/_test/mail` and `/_test/supabase/`, only when `DANDORI_DEV_NOW` is set.
- `internal/app/plan_happens_test.go` — the canonical Seam-1 example: the plan happens in real time, a dinner's second portion is eaten two days later.

### Features

Package-by-feature slices (auth, ingredients, kitchen, planning, profile, recipes, runs, today), each split into prefixed tier files: web handlers, messages and forms, a Service and a store.

- `internal/ingredients/service.go` — adds, edits, lists, searches and deletes the User's Ingredients; refuses names taken and units fixed by Movements.
- `internal/kitchen/service.go` — reads what the kitchen holds from the Ledger, records Counts, moves and Portion additions as Movements in one transaction.
- `internal/planning/service_plan.go` — builds the prompt and brief the planner Run is given, judges its draft Plan against the hard rules, stores the Plan Proposal.
- `internal/planning/service_ledger.go` — brings the kitchen up to now, decides Plan Proposals, starts the replanning Runs.
- `internal/recipes/service_parse.go` — the parse Run's shape: the Recipe the model submits, how it is checked against the User's Ingredients, the prompt and tool schema.
- `internal/runs/service_engine.go` — runs agent Runs in the background as their User, records each Run and its Trace, lets the User follow or cancel it.
- `internal/today/service.go` — the day's Slots, how up to date the kitchen is, what a Delivery brings, whether to open a Cycle.
- `internal/profile/service.go` — the Profile: Language, Slot times, Targets, Meal Out estimate, exclusions, allergies, appliances and the Planning Brief.

### Kitchen Ledger

The append-only Kitchen Ledger of Movements and Portions, a package below the features that write Counts, moves and Portions (ADR-0019).

- `internal/ledger/service.go` — the write side: holds the signed-in User's kitchen lock, hands a writer the Ledger as it stands, saves the Movements it makes.
- `internal/ledger/store.go` — the SQL: reads and writes Movements, Portions, the kitchen row's `up_to` mark and the scheduler's beat.

### Agent Engine

The hand-rolled LLM agent loop (ADR-0003) with its Brief, Trace, the real Anthropic Claude client and the fakellm test double, a shared engine below the runs, recipes and planning features.

- `internal/agent/agent.go` — the vocabulary of the loop: the LLM port, a request and a response, the blocks of a conversation, the tools offered to the model.
- `internal/agent/brief.go` — what a Run is told: the system prompt, the stable block, the prompt, the tools it may use, the effort.
- `internal/agent/loop.go` — runs a brief against the model, runs the tools it calls, retries the failures worth retrying, records every step.
- `internal/agent/trace.go` — the event kinds the live Trace renders, the Trace that receives them, the Failure a Run ends with.
- `internal/agent/anthropic/anthropic.go` — the real client: maps a request and a brief's tools onto the Beta Messages endpoint and the answer back onto the agent's own types.
- `internal/agent/fakellm/fakellm.go` — a scripted LLM for tests, so a Run goes the same way every time.

### Domain Core

Pure domain types and rules (appliances, Cycles, dates, foods, ingredients and matching, language, ledger arithmetic, meals out, Plans, Portions, Profiles, Recipes, meal slots, soft rules, targets, thawing) tested at Seam-2.

- `internal/domain/doc.go` — the package doc: the pure core, the rules of the model from GLOSSARY.md with no I/O.
- `internal/domain/ledger.go` — the Kitchen Ledger arithmetic in exact tenths: the Movements of the Stock Items with their Causes, and what the kitchen holds at a moment.
- `internal/domain/plan.go` — the Plan model: Prep Sessions and the Portions they yield, the Portions put in Slots, the hard rules a Plan breaks.
- `internal/domain/judge.go` — the Verdict on a draft Plan: the hard rules it breaks, the days it plans, the Targets and soft rules it misses.
- `internal/domain/soft.go` — the soft rules: what a Plan eats each day, how that compares with the User's Targets, the misses shown to weigh.
- `internal/domain/walk.go` — walks the kitchen up to a moment: the Movements a Plan's deliveries, Prep Sessions and Slots write.
- `internal/domain/thaw.go` — frozen food in a Plan: what each Prep Session thaws the evening before, and when there is no evening left to thaw it.
- `internal/domain/cycle.go` — the Cycle one Delivery has to last: its window, its end date, its status, the Slots it holds.

### Web UI

Shared templ components, layout, i18n messages, htmx answers and Notices, icons, Markdown rendering, CSS tokens, Faro script and the static/PWA assets every feature page is built from.

- `internal/web/web.go` — the app shell: static assets, the PWA manifest, the service worker, the Faro collector's URL.
- `internal/web/i18n.go` — the shared i18n layer: the `Message` type every page shows, the Language of the request, the date, time and number formats each Language writes.
- `internal/web/answer.go` — the Answer a write gives: a fragment and a trigger for htmx, a redirect for a plain browser, or the page itself.
- `internal/web/notice.go` — the floating Notice: a message carried across a redirect in a cookie, shown once by the page it lands on.
- `internal/web/static/app.css` — the one stylesheet of the Pixel ledger design system, in tokens → base → components order.
- `internal/web/static/faro.js` — browser telemetry for Grafana Faro: JS errors, web vitals, sessions and htmx requests.

### Data & Persistence

The pgx connection pool that applies the embedded Goose migrations, the row-level-security user queries, and the SQL schema of the PostgreSQL and pgvector tables behind users, kitchens, ingredients, Movements, Portions, Recipes, Runs, Cycles and Plans.

- `internal/db/db.go` — opens the Postgres pool, applies the embedded Goose migrations, traces every query without its arguments.
- `internal/db/user.go` — carries the signed-in User on a context and runs queries as them, so row-level security scopes every read.
- `internal/db/migrations/00002_users.sql` — creates the authenticated role, `current_user_id()` and the `own_rows` policy pattern every owned table copies.
- `internal/db/migrations/00007_movement.sql` — the append-only movement table, the Kitchen Ledger the Stock Items are derived from.
- `internal/db/migrations/00022_plan_proposal.sql` — the planning schema: `cycle_run`, `plan_proposal` (at most one pending per User), `prep_session`, `planned_portion`, `kept_portion`.
- `internal/db/migrations/00033_plan_happens.sql` — the assumed cause kinds (`used_up`, `prepared`, `eaten`), so the Plan happens at its Slots' times.

### Deployment, CI & Config

The Docker image, the Hetzner compose and Caddy deployment, the GitHub Actions pipeline, the pre-commit hook, the provisioning and PR-image scripts and the project tooling config (go.mod, mise, golangci, MCP, env).

- `Dockerfile` — a two-stage build: Go 1.27 builder cross-compiling a static binary, distroless nonroot runtime on port 8080 with a healthcheck.
- `deploy/compose.yaml` — the VPS stack: the app behind Caddy, which holds the TLS certificate and waits for the app to be healthy.
- `deploy/Caddyfile` — Caddy terminating TLS and proxying to the app container.
- `.github/workflows/ci.yml` — the v2 pipeline: `check` (test database, Chromium, committed templ output, `mise run check`), `changes` and `deploy`.
- `mise.toml` — pins Go, templ, deadcode and golangci-lint, loads `.env`, installs the git hooks and defines every task.
- `.golangci.yml` — the linters plus the depguard rules that enforce the package-by-feature layering.
- `scripts/dev.sh` — starts the server against the dev Postgres; with `DANDORI_DEV_NOW` it recreates a per-port travel database.
- `scripts/provision.sh` — the interactive wizard that provisions the VPS, domain, Supabase project and Grafana stack.

### Docs & Decisions

The project vision, glossary, README, agent instructions and guides, the design system and the ADRs that record the architectural decisions behind the code.

- `GLOSSARY.md` — the domain language in eight sections, each entry giving its meaning and the words to avoid.
- `README.md` — the product in one page: the idea, the v2 status, the stack, how to run it with mise.
- `vision.md` — the product vision and the M1/M2 milestones, so you know what is finished and what is still a placeholder.
- `docs/design.md` — the design system, "Pixel ledger": tokens, dithering, pixel art, layout, the Busy bar, Notices, Refusals, accessibility.
- `docs/agents/testing.md` — the two test seams and the guardrail tests.
- `docs/adr/` — 23 ADRs, one per decision; the ones that shape the code most are 0006 (features), 0013/0019 (the Ledger), 0018 (hard vs soft rules), 0021 (planning brings the kitchen up to now).

## Key Concepts

### Two test seams

Issues name their acceptance criteria by seam, and each kind of test lives in one place.

- **Seam-1 — the app over HTTP.** `internal/app/*_test.go`, run through the `internal/apptest` harness. `apptest.Start` boots the whole app in-process against its own freshly migrated Postgres, cloned per test, so a Seam-1 test changes no state another can see. Tests assert on what the User sees; `h.Exec` and `h.Scan` are for the few things no page shows yet. Two layers: `apptest.NewScenario` tells a test as Given/When/Then over the clock (`s.At("Tue 10:00")`, `s.Open("/")`, `s.Sees("Tuesday, November 3")`), and `h.Browser(t)` opens a headless Chromium for the few tests per page that need htmx, CSS and scripts as the User gets them.
- **Seam-2 — the domain core.** `internal/domain/*_test.go`, usually table-driven, calling the rules as pure functions. No database, no clock: the time a rule needs is passed in.
- **Guardrails** are a third family that check the source instead of the behaviour: colours live only in `app.css` tokens, pages show only `web.Message` strings with both translations, the glossary's avoided words stay avoided, a store calls no service and no handler (`internal/web/tiers_test.go`), every table has one owning feature (`internal/web/owners_test.go`), nothing outside `internal/clock` reads the machine's time, and the binary links no test harness and mounts the dev routes only in dev mode.

`mise run check` runs everything CI runs; `mise run guardrails` runs the fast subset in seconds and is what the pre-commit hook runs.

### Package by feature, a Service per feature

ADR-0006 decides that code is grouped by feature (`internal/recipes`, `internal/profile`, `internal/planning`, …) and that each feature package splits into tiers by file prefix: `web_handlers.go`, `web_messages.go`, `web_form.go`, `service*.go`, `store.go`. The Services hold the rules and the refusals; the handlers are thin. The store is unexported and speaks SQL, and it owns its feature's tables — `internal/web/owners_test.go` fails the build if a foreign-key cascade crosses features or a store writes another feature's table.

Shared machinery (`internal/domain`, `internal/agent`, `internal/runs`, `internal/db`, `internal/httpx`, `internal/web`, `internal/clock`, `internal/telemetry`, `internal/apptest`) sits below the features, and only `internal/app/app.go` wires them together. `.golangci.yml` depguard rules make the order mechanical: `internal/domain`, `internal/agent`, `internal/db` and `internal/web` are leaves, `planning` is downstream of everything, `today` is the top.

### The Kitchen Ledger is append-only, and it sits below the features

What the kitchen holds is never stored as such (ADR-0013). Every change is a **Movement** with its time, its cause, and whether it was assumed from the Plan or reported by the User; **Stock Items** and Portions held are computed by adding Movements up to a moment. A correction is a new Movement, never an edit — which is how "why does the fridge hold 450 g of chicken?" stays answerable. Stock Items carry no expiry date (ADR-0011): the User watches for spoiled food and corrects the Stock Item.

ADR-0019 then moved the ledger out of the kitchen feature into `internal/ledger`, below every feature that writes Movements. It owns the `movement`, `portion` and `movement_portion` tables, the kitchen row and the kitchen lock, and it takes a `db.Querier` rather than a pool. What was a write up the graph became a write down it. ADR-0021 puts the other half there: bringing the kitchen up to now lives in `planning`, in one transaction that walks Slot times and Delivery arrivals in time order, taking assumed Movements and arrived Deliveries together under the planning lock and then the kitchen lock.

### The agent proposes, the User approves

A **Plan Proposal** is a set of changes to a Cycle's Plan produced by the agent; it takes effect only when the User approves it. The flow the code implements:

- At most one Plan Proposal is pending, or one planning Run going, at a time, across all Cycles — a partial unique index on `plan_proposal` enforces it in the database, and ADR-0008 records the rule.
- The planner Run submits a draft Plan; `internal/domain/judge.go` returns the **Verdict**: the hard rules it breaks, the days it plans, and the soft misses against the User's Targets.
- ADR-0018 draws the line. Only hard rules stop a Plan Proposal — a Slot filled twice or empty, a Portion eaten before it is prepared, an allergen or exclusion, an appliance the User does not own, frozen food with no evening left to thaw it. Targets and other soft misses are reported to the planner and shown on the proposal for the User to weigh. The sums are done in a `check_plan` tool, not in the model.
- Approving applies the proposal over the Plan it replaces, from the first Slot whose time of day is still to come. Rejecting with a **Planning Note** starts another Run from the rejected one's first Slot, or from the first Slot not yet prepared once that is past; a note without a rejection asks for nothing.
- Recipe Drafts belong to their proposal: accepting it accepts the drafts it prepares, rejecting or replacing it discards them.
- Every agent **Run** keeps a **Trace** of its steps — a model message, a tool call or its result, a retry, a throttle wait — live over SSE and kept after the Run ends.

### One Language per installation

ADR-0015 (superseding ADR-0007) decides that each invited User has their own kitchen and shares nothing with other Users, not even the Known Catalog or Ingredients, and chooses **one Language** for the pages and for every Food Term. `internal/domain/language.go` encodes the one hard edge: the Language can change only until the User has written their first Food Term, after that it stays. The reason is in the model — an Ingredient is identified by its name, so mixing languages would split one food in two.

Everything the User reads is a `web.Message` with an English and an Italian translation (`internal/web/i18n.go`), and `internal/web/i18n_test.go` fails if a translation is missing or loses a verb. Each User's rows are theirs alone: `internal/db/user.go` runs queries as the signed-in User, and every owned table copies the `own_rows` policy from migration `00002_users.sql`, so isolation lives in the database rather than in the query layer.

### Time is an input

The app never calls `now()`. `internal/clock` holds the only clock, and a guardrail test scans every Go and templ file for `time.Now`, `time.Since` or `time.Until` outside it. A manual clock lets a test jump to "Tue 10:00" or a delivery's arrival, and the scheduler beats on that same clock. The same principle appears in the domain: a rule that needs the time takes it as a parameter.

### The ledger arithmetic, the Plan and the Verdict are separate layers

`internal/domain` is pure and carries the highest fan-in in the graph: `ledger.go` does the arithmetic in exact tenths, `plan.go` models Prep Sessions and the Portions they yield, `judge.go` and `soft.go` judge a draft Plan, `walk.go` walks the kitchen up to a moment, and `thaw.go` works out what to thaw the evening before. `internal/planning/service_plan.go` and `service_ledger.go` are where those rules meet the agent and the database. Reading a feature's Service before its handlers is the fastest way into this code.

## Guided Tour

The graph's own learning path, in order. Each step names the files to read.

### 1. Project Overview

Dandori is a meal-planning co-pilot for a few invited users, built on one idea: a dinner yields three portions, one eaten now, one as lunch two days later, one for the freezer. README.md states the product rules that shape the code: planning covers one supermarket delivery at a time (a Cycle), the agent proposes changes and the User approves them, and there is no checkout, the app only fills a cart. vision.md expands the same idea into the milestone plan, so read it to know which parts of the codebase are finished and which are still placeholders.

*Files: `README.md`, `vision.md`.*

### 2. The Domain Language

Before any code, the project fixes its vocabulary: GLOSSARY.md defines Cycle, Delivery, Plan, Slot, Plan Proposal, Verdict and Exception, each with its meaning and the words to avoid. ADR-0008 shows how one of those terms behaves in time, since a Cycle stays upcoming until the end of its Delivery window and only then becomes current, closing the previous Cycle. These words reappear in package names, table names and function names all over the codebase, so reading the glossary first is what makes the rest of the tour legible.

*Files: `GLOSSARY.md`, `docs/adr/0008-cycles-become-current-on-delivery.md`.*

### 3. The Binary and Its Wiring

cmd/dandori/main.go is the whole program: it reads configuration from the environment and either serves the app or answers the container's health check, which is why the Dockerfile can call the binary's own subcommand to check on it. internal/app/app.go is the wiring, opening the database, building every feature's Service and mapping the HTTP routes the browser reaches; it has by far the highest fan-out in the graph, so it is the one file that shows how the features fit together. internal/clock/clock.go holds the only place Dandori reads the time, a real clock and a manual one that tests step, and internal/devmode/devmode.go adds the dev-only time travel with a fake Supabase Auth and a mailbox served under /_test/.

*Files: `cmd/dandori/main.go`, `internal/app/app.go`, `internal/clock/clock.go`, `internal/devmode/devmode.go`.*

### 4. The Pure Domain Core

internal/domain is the pure core of the model, the rules of the glossary with no I/O, so every rule is a function a test can call directly. The files here define the nouns everything else moves around: a Cycle with its Delivery window, the day's Slots, a Plan of Prep Sessions and the Portions they yield, and Portions themselves with their Portion Lots. Because nothing in the package touches Postgres, it carries the highest fan-in in the graph, which is why the planner, the ledger and every feature depend on it.

*Files: `internal/domain/doc.go`, `internal/domain/cycle.go`, `internal/domain/slot.go`, `internal/domain/plan.go`, `internal/domain/portion.go`.*

### 5. The Kitchen Ledger

What the kitchen holds is never stored as such. ADR-0013 records the decision that every change is an append-only Movement with its time, its cause and whether it was assumed from the Plan or reported by the User, and that Stock Items are computed by adding Movements up to a moment, so a correction is a new Movement and never an edit. ADR-0019 then moves the ledger out of the kitchen feature into a package of its own, below every feature that writes Movements, which is why what was a write up the graph became a write down it. internal/domain/ledger.go holds the arithmetic in exact tenths, internal/ledger/service.go takes the User's kitchen lock and saves Movements, and internal/ledger/store.go is the SQL.

*Files: `internal/domain/ledger.go`, `internal/ledger/service.go`, `internal/ledger/store.go`, `docs/adr/0013-the-kitchen-is-an-append-only-ledger.md`, `docs/adr/0019-the-kitchen-ledger-sits-below-the-features-that-write-movements.md`.*

### 6. Seam-2: The Domain Rules

This is the first of the project's two test seams. Seam 2 sits beside the code it tests in internal/domain, where the files ending in _test.go are external-package tests that call the rules as pure functions: parsing quantities, adding Movements up, judging a draft Plan's hard rules, opening a Cycle from typed fields. docs/agents/testing.md explains the whole scheme, with Seam 1 driving the app over HTTP, Seam 2 testing the domain core as pure functions, and a third family of guardrail tests checking the source instead of the behaviour. Reading one of these files next to the domain file it covers is the fastest way to learn what a rule actually guarantees.

*Files: `internal/domain/ledger_test.go`, `internal/domain/plan_test.go`, `internal/domain/cycle_test.go`, `docs/agents/testing.md`.*

### 7. The Schema in Goose Migrations

The schema is the numbered files under internal/db/migrations, applied by Goose at startup from Go's embed, so the database's history has the same shape as the code's history. Read them in number order and you read the product's data model chronologically: users and their row-level security first, then the append-only movement table, then the cycle table whose status is computed rather than stored, then the planning tables with a partial unique index allowing only one pending Plan Proposal per User, and finally the fixed Grocery List lines written when a Delivery is marked ordered. The table-level nodes in this tour point at the same migrations for the individual tables, so a table's meaning is readable next to the DDL that created it.

*Files: `internal/db/migrations/00002_users.sql`, `internal/db/migrations/00007_movement.sql`, `internal/db/migrations/00021_cycle.sql`, `internal/db/migrations/00022_plan_proposal.sql`, `internal/db/migrations/00030_grocery_line.sql`.*

### 8. Every User Sees Own Rows

Multi-tenant isolation lives in the database rather than in the query layer: the users migration creates an authenticated role, a current_user_id() helper that reads the JWT claims, and an own_rows policy pattern that every owned table copies. internal/db/user.go carries the signed-in User on a context and runs queries as them, so row-level security does the scoping for free, and internal/db/db.go opens the pgx pool, applies the embedded migrations and traces every query without its arguments. internal/auth/service.go is the other half of the story, magic links through Supabase Auth and the User created at their first sign-in, and ADR-0015 records that each invited User shares nothing at all, not even the Known Catalog.

*Files: `internal/db/user.go`, `internal/db/db.go`, `internal/auth/service.go`, `internal/db/migrations/00002_users.sql`, `docs/adr/0015-users-share-nothing-and-choose-their-language.md`.*

### 9. Features as Package Slices

ADR-0006's layout seen in code: each feature is a package split into prefixed tier files, web handlers, messages and forms, a Service and an unexported store, with shared machinery below the features and only app.go wiring them together. The Services show what each feature is really about: internal/ingredients/service.go refuses names that are taken and units fixed by movements, internal/kitchen/service.go records Counts, moves and Portion additions as Movements in one transaction, internal/today/service.go answers the day's Slots and how up to date the kitchen is, internal/recipes/service.go is the Recipe Library and internal/planning/service.go opens and drops Cycles. Read a feature's Service before its handlers, because the handlers are thin and the Service holds the rules and the refusals.

*Files: `internal/ingredients/service.go`, `internal/kitchen/service.go`, `internal/today/service.go`, `internal/recipes/service.go`, `internal/planning/service.go`.*

### 10. Planning a Cycle

Planning is where the domain, the agent and the database meet. internal/planning/service_plan.go builds the prompt and brief the planner Run is given, judges its draft Plan against the hard rules and stores the Plan Proposal it submits, while internal/planning/service_ledger.go brings the kitchen up to now, decides proposals and starts the replanning Runs. internal/domain/judge.go is the Verdict, the hard rules a Plan breaks, the days it plans and the soft misses against the User's Targets, and internal/domain/grocery.go computes what a Plan still needs to buy against the stock and where a Delivery puts its lines away. internal/planning/web_handlers.go is where the User sees all of it: the Cycle page, the Kitchen Check, approving or rejecting a proposal with a note, and the Grocery List.

*Files: `internal/planning/service_plan.go`, `internal/planning/service_ledger.go`, `internal/planning/web_handlers.go`, `internal/domain/judge.go`, `internal/domain/grocery.go`.*

### 11. The Agent Engine

ADR-0003 records the decision to write the agent's tool-use loop by hand against the Anthropic API instead of using a framework, so that every step is naturally a trace event. internal/agent/brief.go is what a Run is told, the system prompt, the stable block, the prompt, the tools it may use and the effort, and internal/agent/agent.go holds the port itself: the LLM interface, the request and response, the conversation blocks and the tools offered to the model. internal/agent/loop.go runs the brief against the model, runs the tools it calls, retries the failures worth retrying and records every step, and internal/agent/trace.go defines the event kinds the live trace renders, while internal/agent/anthropic/anthropic.go is the real client over the Claude API.

*Files: `internal/agent/agent.go`, `internal/agent/loop.go`, `internal/agent/brief.go`, `internal/agent/trace.go`, `docs/adr/0003-hand-rolled-agent-loop-and-mcp-client.md`.*

### 12. The Shared Web Shell

ADR-0002 records the decision to build the UI as server-rendered templ pages driven by htmx, with SSE for the live agent trace, on a Go backend, so the app stays a single deployable binary and a PWA. internal/web/web.go is the shell, with the static assets, the PWA manifest, the service worker and the Faro collector's URL, and internal/web/i18n.go is the shared i18n layer, the Message type every page shows, the Language of the request and the date, time and number formats each Language writes. internal/web/answer.go is the Answer a write gives, a fragment and a trigger for htmx, a redirect for a plain browser or the page itself, and internal/web/notice.go is the floating Notice carried across a redirect in a cookie. Every feature page is built from these components, so one feature's templates teach the whole UI vocabulary.

*Files: `internal/web/web.go`, `internal/web/i18n.go`, `internal/web/answer.go`, `internal/web/notice.go`, `docs/adr/0002-go-templ-htmx-frontend.md`.*

### 13. Seam-1: The apptest Harness

This is the other seam: Seam 1 drives the whole app over HTTP against a real, freshly migrated Postgres of its own. internal/apptest/apptest.go starts Dandori in-process with a manual clock the test steps, a fake Auth, a scripted model and the HTTP helpers the tests use, while internal/apptest/scenario.go turns a test into Given/When/Then over the app's clock and keeps a timeline a failed test prints, and internal/apptest/browser.go drives a real headless Chromium for the tests that need the UI as the User gets it. internal/app/plan_happens_test.go is a good example, checking that the plan happens in real time because opening a page or the beat brings the plan up to now and a dinner's second portion is eaten two days later, and internal/agent/fakellm/fakellm.go answers each model call with the next scripted step so a Run goes the same way every time.

*Files: `internal/apptest/apptest.go`, `internal/apptest/scenario.go`, `internal/apptest/browser.go`, `internal/app/plan_happens_test.go`, `internal/agent/fakellm/fakellm.go`.*

### 14. Deployment and CI

ADR-0017 records the decision to host v2 as one binary on a small Hetzner VPS in Germany behind Caddy with Docker Compose, deployed by GitHub Actions on every merge to v2, because planning Runs are long background loops and the app has to stay up rather than scale to serverless containers. The Dockerfile is a two-stage build, a Go 1.27 builder cross-compiling a static ./cmd/dandori binary and a distroless nonroot runtime image that ships only that binary on port 8080 with a healthcheck. deploy/compose.yaml runs the app behind Caddy, which holds the TLS certificate and waits for the app to be healthy, and .github/workflows/ci.yml is the pipeline for the v2 branch, with a check job that brings up a test database and Chromium for the browser tests, checks the committed templ output and runs mise run check.

*Files: `Dockerfile`, `deploy/compose.yaml`, `deploy/Caddyfile`, `.github/workflows/ci.yml`, `docs/adr/0017-hosted-on-hetzner-with-docker-compose.md`.*

## File Map

### Entry & Runtime Wiring

| File | What it does |
|---|---|
| `cmd/dandori/main.go` | The binary: reads the environment, serves the app, or answers the health check. |
| `cmd/dandori/main_test.go` | The binary links no test harness; the dev routes mount only in dev mode. |
| `internal/app/app.go` | Opens the database, builds every feature's Service, maps every HTTP route. |
| `internal/clock/clock.go` | The only clock: real and manual, both able to beat on a schedule. |
| `internal/clock/clock_test.go` | Guardrail: nothing outside `internal/clock` reads the machine's time. |
| `internal/httpx/httpx.go` | Shared HTTP helpers: path ids, htmx detection, rendering, refusals on the span. |
| `internal/telemetry/telemetry.go` | OTLP span and log exporters, and the slog logger with span and User ids. |

### Seam-1 Tests & Harness

| File | What it does |
|---|---|
| `internal/apptest/apptest.go` | Boots the whole app in-process over HTTP on its own migrated Postgres. |
| `internal/apptest/scenario.go` | Given/When/Then over the app's clock, with a timeline on failure. |
| `internal/apptest/browser.go` | Headless Chromium for the tests that need the UI as the User gets it. |
| `internal/apptest/form.go` | Reads a form to post, the way a browser would. |
| `internal/apptest/link.go` | Follows a link, the way a browser would. |
| `internal/apptest/text.go` | The page's main content without markup. |
| `internal/apptest/signin.go` | Signs a User in through the pages and the emailed magic link. |
| `internal/apptest/fakeauth.go` | The fake Supabase Auth beside the app in a Seam-1 test. |
| `internal/devmode/devmode.go` | Mounts the `/_test/` clock, seed, mailbox and fake Supabase routes in dev mode. |
| `internal/devmode/fakeauth.go` | The dev fake Auth: invites Users, mails magic links, signs sessions with its own key. |
| `internal/devmode/seed.go` | Seeds a dev database through the app's own shapes. |
| `internal/app/plan_happens_test.go` | The plan happens in real time; a second portion is eaten two days later. |
| `internal/app/today_test.go` | The Today page: the Slot now or next, how planning stands, what a new User sees. |
| `internal/app/browser_test.go` | Through a real browser: the clock's day, magic-link sign-in, the Language choice. |
| `internal/app/spans_test.go` | Every request, beat and Run is a span, saying who the User was and holding no secret. |

### Features

| File | What it does |
|---|---|
| `internal/auth/service.go` | Signs Users in and out through Supabase Auth's magic links. |
| `internal/auth/store.go` | Creates the User at their first sign-in; reads their email back. |
| `internal/auth/store_supabase.go` | The Supabase Auth HTTP API: magic links, session refresh and revoke, JWKS verification. |
| `internal/auth/web_handlers.go` | Sign-in, sign-out and the confirmation page; the session in cookies. |
| `internal/auth/web_messages.go` | The bilingual messages of the sign-in pages. |
| `internal/ingredients/service.go` | Add, edit, list, search and delete Ingredients; refuse names taken and units fixed. |
| `internal/ingredients/store.go` | The Ingredients SQL, relying on row-level security for scope. |
| `internal/ingredients/web_handlers.go` | The Ingredients page, its side sheet and the fragments htmx swaps. |
| `internal/ingredients/web_form.go` | The add and edit forms as typed, and why each field was refused. |
| `internal/kitchen/service.go` | Reads the kitchen from the Ledger; records Counts, moves and Portions in one transaction. |
| `internal/kitchen/web_handlers.go` | The Kitchen page and its sheets: counting, adding and counting Portions, moving stock. |
| `internal/kitchen/web_form.go` | The Count, move, Portion Lot and Add Portions forms as shown. |
| `internal/kitchen/web_messages.go` | The bilingual Kitchen messages, including why a quantity is refused. |
| `internal/planning/service.go` | Opens, drops and finds Cycles; says how planning each one stands. |
| `internal/planning/service_plan.go` | The planning Run: prompt and brief, the hard rules, the stored Plan Proposal. |
| `internal/planning/service_ledger.go` | Brings the kitchen up to now, decides proposals, starts the replanning Runs. |
| `internal/planning/service_grocery.go` | The Grocery List: what the Delivery buys, marking it ordered, where lines go. |
| `internal/planning/store.go` | The SQL for Cycles, Plan Proposals, Prep Sessions, Planning Notes and drafts. |
| `internal/planning/web_handlers.go` | The Cycle page: open and drop, Kitchen Check, approve or reject, Grocery List. |
| `internal/planning/web_messages.go` | The bilingual messages of the Cycle, Proposal, Kitchen Check and Grocery List. |
| `internal/profile/service.go` | The Profile: Language, Slot times, Targets, Meal Out, exclusions, appliances, brief. |
| `internal/profile/store.go` | The SQL for the Profile row. |
| `internal/profile/web_handlers.go` | The Language choice and the Profile page; saves every field on its own. |
| `internal/profile/web_messages.go` | The bilingual Profile catalogue, with the wording for each refusal. |
| `internal/recipes/service.go` | The Recipe Library: add, edit, delete, search Recipes and their Ingredients. |
| `internal/recipes/service_parse.go` | The parse Run's shape: the submitted Recipe, the check against Ingredients, the prompt. |
| `internal/recipes/service_draft.go` | The User's Recipe Drafts: starts the parse Run, accepts or discards. |
| `internal/recipes/service_drafted.go` | The planner's Recipe Drafts, owned by a Plan Proposal. |
| `internal/recipes/service_demo_llm.go` | A stand-in for the model, so the pages can be tried without an LLM. |
| `internal/recipes/store.go` | The SQL for Recipes, their Ingredients and the drafts their Runs leave. |
| `internal/recipes/web_handlers.go` | The Recipe Library and form: paste for a parse Run, check the draft, edit, delete. |
| `internal/recipes/web_form.go` | The Recipe form as shown: the typed Recipe, its rows, why a field was refused. |
| `internal/runs/service_engine.go` | Runs agent Runs in the background as their User; records each Run and its Trace. |
| `internal/runs/service_spans.go` | Makes each model call a client span under the Run's, per the gen-AI conventions. |
| `internal/runs/store.go` | Writes a Run, how it ended and its Trace Events; reads the User's Runs back. |
| `internal/runs/web_handlers.go` | The Runs list, a Run's page with its Trace, the live SSE stream, cancelling a Run. |
| `internal/today/service.go` | The day's Slots, how up to date the kitchen is, what a Delivery brings, open a Cycle? |
| `internal/today/web_handlers.go` | The day at the clock's now, and the put-away list Today swaps in after a write. |
| `internal/today/web_messages.go` | Today's fixed lines, and what each Offer and Standing is called. |

### Kitchen Ledger

| File | What it does |
|---|---|
| `internal/ledger/service.go` | The write side: the User's kitchen lock, the Ledger as it stands, saving Movements. |
| `internal/ledger/store.go` | The SQL for Movements, Portions, the kitchen's `up_to` mark and the scheduler's beat. |

### Agent Engine

| File | What it does |
|---|---|
| `internal/agent/agent.go` | The LLM port, request and response, conversation blocks, the tools offered. |
| `internal/agent/brief.go` | What a Run is told: prompt, stable block, tools, effort. |
| `internal/agent/loop.go` | The tool-use loop: runs the model, runs the tools, retries, records every step. |
| `internal/agent/trace.go` | The Trace's event kinds, the Trace itself, and the Failure a Run ends with. |
| `internal/agent/anthropic/anthropic.go` | The Claude adapter over the Beta Messages endpoint. |
| `internal/agent/fakellm/fakellm.go` | The scripted LLM, so a Run goes the same way every time. |

### Domain Core

| File | What it does |
|---|---|
| `internal/domain/doc.go` | The package doc: the pure core, the rules of the model with no I/O. |
| `internal/domain/ledger.go` | Quantities in exact tenths, the Movements with their Causes, what the kitchen holds. |
| `internal/domain/plan.go` | Prep Sessions, the Portions they yield, the Portions in Slots, the hard rules. |
| `internal/domain/judge.go` | The Verdict: hard rules broken, days planned, Targets and soft rules missed. |
| `internal/domain/soft.go` | What a Plan eats each day against the User's Targets, and the misses shown. |
| `internal/domain/walk.go` | Walks the kitchen up to a moment: the Movements a Plan writes. |
| `internal/domain/forecast.go` | The kitchen forecast at a Slot: stock, the Plan's arrivals, Portions without a Slot. |
| `internal/domain/grocery.go` | What a Plan needs beyond the stock, and where a Delivery puts its lines away. |
| `internal/domain/thaw.go` | What thaws the evening before, and when there is no evening left. |
| `internal/domain/changes.go` | What a Plan Proposal changes of the Plan it replaces. |
| `internal/domain/replace.go` | What a proposal replaces and keeps of a Plan, and how approval applies it. |
| `internal/domain/cycle.go` | The Cycle, its Delivery window, its end date, its status, its Slots. |
| `internal/domain/slot.go` | The kinds of eating occasion, their times of day, the day's Slots now and next. |
| `internal/domain/portion.go` | Portions and Portion Lots: reading them, grouping them, counting them down. |
| `internal/domain/ingredient.go` | An Ingredient as a food: name, key, unit, storage location, nutrition. |
| `internal/domain/ingredient_match.go` | Finds the known Ingredient a mistyped name meant; which names a search finds. |
| `internal/domain/recipe.go` | The Recipe model: reading it as typed, what is wrong with it, a Portion's nutrition. |
| `internal/domain/target.go` | The daily Targets, read as typed, each field refused on its own. |
| `internal/domain/meal_out.go` | The Meal Out estimate, parsed field by field with its own problem per bad value. |
| `internal/domain/profile.go` | The Profile type: Language, Slot times, Targets, exclusions, appliances. |
| `internal/domain/appliance.go` | The fixed list of appliance kinds (ADR-0020) and how the owned ones are read. |
| `internal/domain/language.go` | The one Language a User speaks; it can change only until the first Food Term. |
| `internal/domain/foods.go` | The Profile's exclusions and allergies, normalised from a typed list. |
| `internal/domain/datetime.go` | Times of day and dates written and read in the User's Language. |

### Web UI

| File | What it does |
|---|---|
| `internal/web/web.go` | The shell: static assets, PWA manifest, service worker, the Faro collector's URL. |
| `internal/web/i18n.go` | The `Message` type, the Language of the request, the date, time and number formats. |
| `internal/web/messages.go` | The shell's message catalogue and the shared naming and quantity helpers. |
| `internal/web/answer.go` | The Answer a write gives: a fragment for htmx, a redirect, or the page. |
| `internal/web/notice.go` | The floating Notice, carried across a redirect in a cookie. |
| `internal/web/icons.go` | The app's pixel icons, each a 10×10 bitmap drawn as inline SVG. |
| `internal/web/pixels.go` | The README banner's pixel art, drawn from bitmaps of CSS tokens. |
| `internal/web/markdown.go` | Renders an agent's Markdown as HTML in a block of class `prose`. |
| `internal/web/static/app.css` | The one stylesheet: the `:root` tokens, base styles and every component. |
| `internal/web/static/faro.js` | Browser telemetry: JS errors, web vitals, sessions and htmx requests. |
| `internal/web/root/manifest.webmanifest` | Makes Dandori installable: name, display, theme colours, icons. |
| `internal/web/root/sw.js` | Service worker: makes the app installable, sends everything to the network. |

### Data & Persistence

| File | What it does |
|---|---|
| `internal/db/db.go` | Opens the pgx pool, applies the embedded Goose migrations, traces every query. |
| `internal/db/user.go` | Carries the signed-in User on a context and runs queries as them. |
| `internal/db/migrations/00001_enable_pgvector.sql` | Enables the pgvector extension. |
| `internal/db/migrations/00002_users.sql` | The authenticated role, `current_user_id()` and the `own_rows` policy pattern. |
| `internal/db/migrations/00003_profile.sql` | The per-User Profile row, written at the first Language choice. |
| `internal/db/migrations/00004_kitchen.sql` | The kitchen row: `up_to` and the scheduler's `beaten_at`, neither moving back. |
| `internal/db/migrations/00006_ingredient.sql` | The per-User Ingredient, with its normalised key and per-unit nutrition. |
| `internal/db/migrations/00007_movement.sql` | The append-only movement table, with cause kinds and believed/stated counts. |
| `internal/db/migrations/00011_portion.sql` | Portions, ready-to-eat servings with their own nutrition, and the join table. |
| `internal/db/migrations/00012_recipe.sql` | Recipes and recipe_ingredient, names unique per User. |
| `internal/db/migrations/00014_run.sql` | `run`, `trace_event` and `recipe_draft`: the agent's Runs and their Traces. |
| `internal/db/migrations/00016_targets.sql` | The daily Targets: floors, a ceiling and the kcal tolerance. |
| `internal/db/migrations/00021_cycle.sql` | The Cycle with its Delivery window; the status is derived, not stored. |
| `internal/db/migrations/00022_plan_proposal.sql` | `cycle_run`, `plan_proposal` (one pending per User), `prep_session`, `planned_portion`. |
| `internal/db/migrations/00023_planning_note.sql` | The Planning Note the User leaves when rejecting a proposal. |
| `internal/db/migrations/00025_freezer_split.sql` | How much of an Ingredient the Delivery puts in the freezer, in order. |
| `internal/db/migrations/00026_planner_recipe_draft.sql` | The Recipes the planner drafts, owned by their Plan Proposal. |
| `internal/db/migrations/00030_grocery_line.sql` | The Cycle's fixed Grocery List, written when the Delivery is marked ordered. |
| `internal/db/migrations/00031_put_away.sql` | The `put_away_at` mark and the `delivered` cause kind. |
| `internal/db/migrations/00033_plan_happens.sql` | The assumed causes `used_up`, `prepared` and `eaten`. |

### Deployment, CI & Config

| File | What it does |
|---|---|
| `Dockerfile` | Two-stage build to a distroless image shipping one binary on port 8080. |
| `deploy/compose.yaml` | The VPS stack: the app behind Caddy. |
| `deploy/Caddyfile` | TLS termination and the proxy to the app container. |
| `.github/workflows/ci.yml` | The v2 pipeline: `check`, `changes` and `deploy`. |
| `.githooks/pre-commit` | Runs `mise run guardrails` before every commit. |
| `mise.toml` | Tool pins, `.env` loading, hook install and every task. |
| `go.mod` | Go 1.27.3 with templ, the Anthropic SDK, pgx, goose, OTel, Playwright and goldmark. |
| `.golangci.yml` | The linters plus the depguard rules enforcing the layering. |
| `.env.example` | The runtime settings the app reads, from the database URL to the Faro URL. |
| `.mcp.json` | The chrome-devtools and Grafana Cloud MCP servers for agent tooling. |
| `scripts/dev.sh` | Runs the server against the dev Postgres; with `DANDORI_DEV_NOW`, a travel database. |
| `scripts/ships.sh` | Decides whether the commits between two revisions change what a deploy ships. |
| `scripts/provision.sh` | The interactive wizard provisioning the VPS, domain, Supabase and Grafana. |
| `scripts/pr-image.sh` | Publishes PR images on the orphan `pr-images` branch and prints the `<img>` tags. |
| `scripts/wrap/main.go` | Rewraps the Go comments that break the 90-column rule. |

### Docs & Decisions

| File | What it does |
|---|---|
| `GLOSSARY.md` | The domain language in eight sections, with the words to avoid. |
| `README.md` | The product in one page: idea, v2 status, stack, how to run it. |
| `vision.md` | The product vision and the milestone plan, so you know what is finished. |
| `AGENTS.md` | How work happens here: branches off `v2`, the two seams, the design system, the tooling. |
| `docs/design.md` | The "Pixel ledger" design system: tokens, components, accessibility baseline. |
| `docs/agents/testing.md` | The two test seams, the guardrails and the mise tasks that run them. |
| `docs/agents/coding-standards.md` | The rules code review enforces. |
| `docs/agents/dev-mode.md` | The manual clock, the `/_test/` routes, the fake Auth and the mailbox. |
| `docs/agents/browser-tools.md` | The chrome-devtools and playwright-brave MCP servers, and the dev-server rules. |
| `docs/agents/grafana.md` | Where the spans, logs and Faro telemetry land, and the privacy rules. |
| `docs/agents/domain.md` | How agents consume the glossary and the ADRs. |
| `docs/agents/issue-tracker.md` | GitHub issue and PR conventions with the `gh` CLI. |
| `docs/agents/triage-labels.md` | The five canonical triage labels and their GitHub strings. |
| `docs/adr/0006-package-by-feature-with-a-service-per-feature.md` | The three tiers per feature and the machinery below them. |
| `docs/adr/0008-cycles-become-current-on-delivery.md` | A Cycle's status over time and one pending proposal at a time. |
| `docs/adr/0013-the-kitchen-is-an-append-only-ledger.md` | Nothing is stored as stock; corrections are new Movements. |
| `docs/adr/0014-v2-is-a-new-codebase.md` | v2 is new, `main` is frozen, time is an input. |
| `docs/adr/0015-users-share-nothing-and-choose-their-language.md` | One User, one kitchen, one Language. |
| `docs/adr/0018-plans-aim-at-targets-only-hard-rules-stop-them.md` | What stops a Plan Proposal and what only gets shown. |
| `docs/adr/0019-the-kitchen-ledger-sits-below-the-features-that-write-movements.md` | The ledger becomes its own package, below the features. |
| `docs/adr/0021-planning-brings-the-kitchen-up-to-now.md` | One transaction, under the planning lock and then the kitchen lock. |
| `docs/adr/0022-dev-time-travel-runs-the-app-on-a-manual-clock.md` | The dev manual clock, fake Auth and `/_test/` routes. |

## Complexity Hotspots

Sixty-one file-level nodes are marked `complex`. Approach them with the size, the branching and the coupling in mind, not by reading them top to bottom.

### The planner and the plan proposal (the deepest coupling)

- `internal/planning/service_plan.go` (1,151 lines) — the largest Go file in the repo: prompt building, the Brief, the hard rules, the Verdict, storing the Plan Proposal and the Recipe Drafts it prepares. Every change here reaches the agent, the domain and three tables.
- `internal/planning/service_ledger.go` (538) — the step that brings the kitchen up to now, decides proposals and starts replanning Runs: it holds the planning lock and then the kitchen lock, so a mistake here corrupts stock as well as plans.
- `internal/planning/store.go` (630) — the SQL for Cycles, proposals, Prep Sessions, Planning Notes and drafts; the tables whose foreign keys and partial unique indexes carry the rules.
- `internal/planning/web_handlers.go` (543) — the Cycle page end to end: open, drop, Kitchen Check, approve, reject, Grocery List.
- `internal/domain/soft.go` (415) — the soft rules: what a Plan eats each day against the Targets, and the misses shown. The hard/soft line ADR-0018 draws is split across this file and `internal/domain/judge.go`.
- `internal/domain/plan.go` (309) — the Plan model everything is judged against; changing it changes every feature above.

### The Kitchen Ledger

- `internal/domain/ledger.go` (454) — the arithmetic in exact tenths and the ordering rules on Movements; the foundation every count, move and plan rests on.
- `internal/ledger/store.go` (269) — the Ledger's SQL, including the `up_to` mark and the scheduler's beat: the write path that must stay append-only.
- `internal/kitchen/service.go` (402) — reads the kitchen from the Ledger and records Counts, moves and Portions as Movements in one transaction; the boundary between the feature and the ledger below it.
- `internal/kitchen/web_handlers.go` (508) — the Kitchen page and its sheets, the busiest handler file in the app.

### The agent and the model

- `internal/agent/anthropic/anthropic.go` (238) — the Claude adapter: request mapping, tool schemas, cache breakpoints, fallbacks and the classification of failures.
- `internal/agent/anthropic/anthropic_test.go` (303) — the adapter's tests against a fake API; they pin the failure classification the loop retries on.
- `internal/runs/service_engine.go` (430) — the background Run: lifecycle, cancellation, the Trace, and the User's ownership of it.
- `internal/recipes/service_parse.go` (285) — the parse Run's prompt, tool schema and the check of the submitted Recipe against the User's Ingredients.
- `internal/app/parse_test.go` (624) — the parse Run end to end: refusals, retries, the turn limit, and what the model is sent back to fix.

### The domain core

- `internal/domain/soft.go` (415) and `internal/domain/soft_test.go` (275) — daily nutrition against the Targets and the misses shown to the User; the largest soft-rule surface.
- `internal/domain/ledger.go` (454) and `internal/domain/ledger_test.go` (484) — the Ledger and its external-package tests; the pair to read side by side.
- `internal/domain/portion_test.go` (321) — Portions, Portion Lots and how they follow the Ledger's Movements.
- `internal/domain/walk_test.go` (346) — walking the kitchen up to a moment: what a Prep Session uses up and prepares.
- `internal/domain/judge_test.go` (338) — the Verdict's verdicts, including the days and the seven before.
- `internal/domain/ingredient_stress_test.go` (248) — the matcher against real shopping lists in English and Italian, with slips and typos.

### Feature services, handlers and stores

- `internal/ingredients/service.go` (338) — refusals on names taken and units fixed by Movements; the validation rules of the feature.
- `internal/ingredients/web_handlers.go` (372) — the Ingredients page, its side sheet and the htmx fragments.
- `internal/recipes/store.go` (252), `internal/recipes/web_form.go` (336), `internal/recipes/web_handlers.go` (509) — the Recipe Library's SQL, its form model and its handlers; a Recipe touches Ingredients, Portions and drafts at once.
- `internal/runs/web_handlers.go` (265) — the Runs list, a Run's page, the live SSE Trace stream and cancellation.
- `internal/profile/web_handlers.go` (353) — the Language choice and every Profile field, each saved on its own.
- `internal/planning/service.go` (238), `internal/planning/web_messages.go` (272) — the Cycle workflow and the bilingual wording of the Cycle, Proposal, Kitchen Check and Grocery List pages.
- `internal/auth/store_supabase.go` (296) — the Supabase Auth API: magic links, session refresh and revoke, JWKS verification. The trust boundary; handle failures deliberately.
- `internal/devmode/fakeauth.go` (351) — the dev fake Auth, which signs sessions with its own key; a change here can quietly make the dev server behave unlike production.

### The wiring and the harness

- `internal/app/app.go` (348) — the highest fan-out in the graph: every Service is built and every route mapped here. Adding a feature means touching this file; a mistake here breaks unrelated features.
- `internal/apptest/scenario.go` (713) — the Given/When/Then DSL every Seam-1 test is written in; a change to the DSL's timing semantics silently changes every test using it.
- `internal/apptest/apptest.go` (382) — boots the app in-process on a cloned database; the harness's own conventions (per-test clone, parallel safety, no `t.Setenv`) are what keep the suite honest.
- `internal/apptest/browser.go` (330) — the headless Chromium driver and the `/_test/clock` switch the browser tests move time with.
- `internal/devmode/fakeauth.go` — see above; dev-only, but it is the seam every fake-Auth test trusts.

### The long Seam-1 test files

These are long because a Seam-1 test covers a whole User journey over HTTP and the clock. They are the specification in executable form; read the names, not the bodies.

- `internal/app/recipes_test.go` (1,016) — the Recipe Library and the recipe form, in Seam-1 and browser tests.
- `internal/app/ingredients_test.go` (974) — the Ingredients page end to end, including the rule that fixes an Ingredient's unit.
- `internal/app/count_test.go` (903) — the kitchen count: what it holds, a refused count, staples, and the browser flow.
- `internal/app/spans_test.go` (722) — asserts on the shape of every span; sensitive to any change in tracing.
- `internal/app/portions_test.go` (660) — adding, counting and moving Portions between storage lots.
- `internal/app/planning_test.go` (648) — planning a Cycle: the Kitchen Check, the proposal, the freezer splits, and a run refused or cancelled.
- `internal/app/parse_test.go` (624) — the parse Run; see the agent group above.
- `internal/app/movements_test.go` (460) — a Stock Item's Movements and why it holds what it does.
- `internal/app/decide_test.go` (439) — approving, rejecting with a note, and a failed Run offering to plan again.
- `internal/app/move_test.go` (416) — moving stock between Storage Locations.
- `internal/app/drafts_test.go` (388) — accepting, refusing and discarding Recipe Drafts.
- `internal/app/changes_test.go` (322) — asking for changes, replanning from the first unprepared Slot.
- `internal/app/planner_drafts_test.go` (323) — the planner's Recipe Drafts on the proposal: shown, accepted or discarded with it, and the approvals refused when a draft no longer fits.
- `internal/app/sheets_test.go` (311) — the bottom sheets, in a real browser.
- `internal/app/cycle_test.go` (304) — opening a Cycle and two Users never seeing each other's.
- `internal/app/tendays_test.go` (412) — a ten-day Cycle's proposal through a browser.
- `internal/app/browser_test.go` (482) — the browser suite: the clock's day, sign-in, the Language choice, the sheets.
- `internal/app/today_test.go` (251), `internal/app/arrival_test.go` (223), `internal/app/ordered_test.go` (220) — the Today page, a Delivery arriving, and marking a Delivery ordered.

### The design system and the guardrails

- `internal/web/static/app.css` (1,202) — the whole "Pixel ledger" design system in one file, in tokens → base → components order. A change at the top moves every page below it.
- `docs/design.md` (421) — the written system the CSS implements; the two must be read together.
- `internal/web/glossary_test.go` (236) — fails the build when pages or code names use a word the glossary avoids: a change to any template can trip it.
- `internal/web/owners_test.go` (270) — fails the build when a table gets a second owning feature or a foreign-key cascade crosses features: a change to any migration can trip it.

### The tools around the app

- `scripts/provision.sh` (418) — the interactive cloud wizard: it writes `.env.provision` and GitHub secrets and variables, so read it before running it.
- `GLOSSARY.md` (220) — the vocabulary every layer is written in; the words to avoid are enforced by a test, so changing them means changing the code that uses them.
