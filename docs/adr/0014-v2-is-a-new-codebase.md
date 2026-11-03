# v2 is a new codebase on its own branch

Using v1 for a real Cycle showed that its core assumptions were wrong in ways that touch everything: the kitchen as mutable state, time measured in days, one user on a local machine. v2 adds Users, a cloud deployment and a Kitchen Ledger, which together reach every table, query and page, so it starts as a new codebase on an orphan branch `v2` instead of being evolved in place. It starts with a walking skeleton that reaches the cloud (sign-in, Profile with Language, an empty Today, scheduler, clock and scenario harness), then adds one feature at a time. `main` is frozen as v1, readable for porting code, and is tagged `v1` when `v2` replaces it. `GLOSSARY.md`, the ADRs and the issues carry over.

## Considered Options

- **Evolve v1 in place**: keeps the agent, Known Catalog and Cart Fill untouched, but tenancy and the ledger would be retrofitted across most of the code while it still has to work.

## Consequences

- Same stack as v1 (Go, templ, htmx 4, Postgres on Supabase, package by feature): code where the model hasn't changed, such as the agent loop and Recipe parsing, is ported with its tests rather than rewritten.
- Time is an input everywhere, never `now()`: scenario tests written in Go step a controlled clock across days and Cycles, and a few Playwright browser tests check each page's main path.
