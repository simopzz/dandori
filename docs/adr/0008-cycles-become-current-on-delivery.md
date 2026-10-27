# Cycles become current when their Delivery arrives

A Cycle is opened days before its Delivery so it can be planned, Cart Filled and ordered. It stays upcoming until the end of its Delivery window, and only then becomes current, closing the previous Cycle and carrying over what is left. Opening a Cycle used to close the current one at once. That froze the last days of the Cycle being lived: Exceptions logged before the bags arrived could not be repaired, and the carry-over was computed from a Plan that had not happened yet. The cost is that an upcoming Cycle is planned against a forecast of what the current one will leave over. Any approved change to the current Plan that changes that forecast leaves the upcoming Plan needing a new Plan Proposal.

To keep that chain from forking, at most one Plan Proposal is pending at a time across all Cycles. A new trigger (an Exception, a Delivery report, a Cart Fill substitution) rejects the pending Plan Proposal with a note, and the next planning Run takes both into account. A current-Cycle Plan Proposal is always decided before the upcoming Cycle is replanned.

## Considered Options

- **Close the current Cycle when the next one is opened** (the first implementation, #22): simple, but its last days can't change and the carry-over is stale.
- **One pending Plan Proposal per Cycle**: a current-Cycle repair and an upcoming-Cycle Plan can both be pending, but approving the repair makes the other one stale anyway.
- **Block Exceptions while a Plan Proposal is pending**: an Exception records something that already happened, so it can't wait.

## Consequences

- A Delivery has a time window, not just a date. It is taken as arrived at the end of the window unless the user reports otherwise, and the window decides which Slots on the delivery day belong to which Cycle.
- Once the Delivery is ordered its products are fixed, so a replan of the upcoming Cycle must make do with what is ordered plus the forecast carry-over.
