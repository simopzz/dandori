# Package by feature, with a service per feature

One `app` package holding every feature's HTTP handlers, SQL and LLM prompts stopped being navigable as features grew, so code is grouped by feature (`internal/recipes`, `internal/profile`, later `internal/planning`, …), each split into the same tiers: handlers (HTTP and form parsing) call a `Service` (use cases in domain types, validation, transaction boundaries), which calls unexported store functions (SQL taking a `db.Querier`, so a caller's transaction can span several stores). Every feature has a `Service`, even a thin one, so each feature looks the same; the feature also owns its templ views and its LLM jobs. Shared machinery sits beside the features and never imports them: `domain` (pure rules), `agent` (the LLM loop), `runs` (the Run engine, Traces and their pages), `db`, `httpx` and `web` (layout, shared components, assets). `app` only wires these together and keeps the one route table, so every URL and its middleware is listed in one place.

## Considered Options

- **Layered packages** (`postgres`, `http`, `jobs` across all features), as in hexagonal architecture: no import cycles possible, but every feature is spread over several packages that all grow with each feature.
- **Keep one `app` package**, split into files by concern: cheapest, but nothing enforces the boundaries.
- **Handlers calling stores directly**, no service: less code today, but transaction rules ended up in handlers, and planning will need use cases that span features.

## Consequences

- Stores are concrete and not behind interfaces; tests hit a real Postgres through `apptest`. Interfaces are kept for external, non-deterministic dependencies (`agent.LLM`, the Playwright MCP client).
- Features may import `runs` and other features' services, but dependencies must stay one-way: if two features need each other, the shared part moves into `domain` or a new package.
