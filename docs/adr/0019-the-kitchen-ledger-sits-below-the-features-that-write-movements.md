# The Kitchen Ledger sits below the features that write Movements

Marking an Ingredient a Staple counts its stock to nothing, a unit is fixed once Movements of it exist, and an Ingredient they refer to can't be deleted: Ingredient writes that follow the Kitchen Ledger's rules, in the Ingredients' transaction, under the kitchen lock. While the Ledger lived in `kitchen`, which imports `ingredients` for names and search, the Ingredients reached it through a struct of three functions `app` wired in, one of which wrote Movements and took the lock, and returned an `ingredients` type. So the Kitchen Ledger is a package of its own, `ledger`, below every feature that writes Movements: it owns the `movement` table, the `portion` and `movement_portion` tables written with a Portion's Movement, the kitchen lock, the `kitchen` row with how far the kitchen is brought up to now and the beat, and the writes and reads of Movements that take a `db.Querier`. `kitchen` keeps the Kitchen page's use cases over Ingredients and the Ledger; `ingredients` imports `ledger` directly.

## Considered Options

- **Flip the direction**, `ingredients` importing `kitchen` and `kitchen` reading Ingredients through functions `app` wires: ADR-0006 allows that, but the Kitchen page's views lean on the Ingredients' events and sheet, which would all move to `web`.
- **Keep the wired functions**: the cycle stays, spread across `app`, and the next feature that writes Movements (planning's assumed ones) would need its own.

## Consequences

- ADR-0006's wired functions are for reads only. A feature that needs to write what another owns, in its own transaction, means the shared part belongs in a package below both.
- Planning, when it writes assumed Movements, imports `ledger` like the Ingredients do.
- `ingredients.Handlers.CountNew` stays a function `app` wires from the Kitchen page's handlers: an edge between handlers, with no transaction or lock in it.
