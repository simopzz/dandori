# Settle the kitchen once per request

Dandori takes a Delivery as arrived, and Prep Sessions as using up their stock, lazily: nothing runs on a timer, so what is stored lags until something brings it up to now. Every use case used to call `planning.Service.Arrive` first, and stock reached it through a function `app` passed in, since `planning` already imports `stock`: a dependency cycle that ADR-0006 rules out, kept only by the order `app` built them in, and a rule held only by a comment, which some use cases already broke. Instead, a `settles` middleware runs `Arrive` once per request, after `needsDatabase`, on every route that needs the database, so use cases read and change what is stored as of now without knowing how it got there. Use cases and agent tools never call `Arrive`. A planning Run's tools read what is stored: what happens while a Run is going reaches the Run the way an Exception does, by replacing its Plan Proposal or cancelling it.

## Considered Options

- **Settling inside `needsDatabase`**: one wrapper per route, but its name would hide half of what it does.
- **Arrival in a kitchen package both features depend on**: arrival closes the current Cycle, rejects its pending Plan Proposals and fixes the Grocery List, so the package would take most of planning with it.
- **Each use case calls `Arrive`**, as before: the rule lives in every use case and is forgotten in some.

## Consequences

- The route table combines the two as `settled(…)`, which every route that needs the database is written with; the route table is the one place to check it. `needsDatabase` and `settles` stay separate middleware, each named for the one thing it does.
- Settling costs a short transaction on every request, also on pages that never show the kitchen, such as the Profile.
- A future clock-driven job (a background worker or a scheduled Run) has to settle first itself.
