# Planning brings the kitchen up to now

Assumed Movements come from the Plan, and a Delivery can arrive between two Slots: Tuesday's breakfast Prep Session must use up what Monday night's Delivery brought. So bringing the kitchen up to now walks Slot times and arrivals in time order, in one transaction, and that step lives in `planning`, which already imports `ledger` and already writes a Delivery's Movements. It holds the planning lock and then the kitchen lock, and writes through `ledger.Write`. `ledger` keeps the kitchen lock, the `kitchen` row (`up_to`, the beat) and the reads and writes of Movements. This amends ADR-0013's "one step", which now also takes Deliveries as arrived, and ADR-0019, under which `ledger` no longer runs the step itself.

## Considered Options

- **Keep the chain** (`kitchen.Handlers.UpToNow`, then `planning.Handlers.Arrived`, each in its own transaction): it can't put a Prep Session after a Delivery that arrived before it.
- **A small package above `planning`** that writes down into `ledger`: one more package for a step only `planning` has the inputs for.

## Consequences

- Every request and the scheduler's beat run one middleware or one beat, `planning`'s, in place of two.
- An assumed Movement is dated at its Slot's time, or at the latest Movement's time if that is later, so it never lands before a Movement it was checked against.
