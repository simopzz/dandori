# The kitchen is an append-only Kitchen Ledger

What the kitchen holds is never stored as such. Every change is an append-only Movement with its time, its cause, and whether Dandori assumed it from the Plan or the User reported it. Stock Items and Portions held are computed by adding Movements up to a moment. In v1 a Stock Item's quantity was overwritten in place and deleted when used up, so "why does the fridge hold 450 g?" could not be answered from the data, though the maths behind it was right. An assumed Movement is written once, when its time passes, as the Plan and its Recipes were then: a ledger computed afresh from the Plan on every read would change its own past whenever a Recipe was edited. Corrections are new Movements (a Count), never edits.

## Considered Options

- **Mutable stock plus a side log** (v1's `stock_taken`): the log and the state can disagree, and nothing reads the log.
- **Assumed Movements computed on read, only reported ones stored**: purely functional, but editing a Recipe rewrites what was used up days ago.
- **Event-sourcing Plans and Cycles too**: a Plan changes only through approved Plan Proposals, each of which stores the whole Plan, so their list already is the Plan's history.

## Consequences

- One step, "bring the kitchen up to now", writes the assumed Movements whose time has passed. It is idempotent: the scheduler runs it every few minutes and every read runs it first, so a page is never wrong because a job ran late, and the scenario harness drives time through it.
- No snapshots: at one User's volume (some 7k Movements a year) adding up from the start costs milliseconds. If it ever doesn't, each Cycle's closing balance is a natural checkpoint to start from, added without touching a Movement.
