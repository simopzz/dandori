# Stock Items have no expiry date

Dandori used to record an optional expiry date on each Stock Item. Prep Sessions used up the earliest expiry first and skipped expired stock, and the Grocery List judged expiry its own way, so the two disagreed. The user watches for spoiled food themselves, so Dandori records no expiry dates. Spoiled stock is corrected on the Stock Item like any other drift. When that leaves a current-Cycle Prep Session short, it is a Shortfall, replanned only when the user asks. The planner still uses first what would spoil first, judged from what the Ingredient is and its Storage Location.

## Considered Options

- **Keep expiry dates and make every computation honour them on each Prep Session's day** (#103): one consistent model, but the user has to type a date for every Stock Item to get it.
- **Log spoiled stock as a discarded Stock Item, an Exception that replans by itself**: once Dandori can forecast Shortfalls, it does the same thing as a correction, so it would be two actions for one event, and fixing a typo would replace a pending Plan Proposal.

## Consequences

- Stock Items are used up oldest first. The Grocery List subtracts stock as one total, which now agrees with use-up.
- A discarded Portion is still an Exception: it empties a Slot, which no other stock can fill.
- The Kitchen Check before planning an upcoming Cycle is where drift, spoilage included, gets corrected, so later Shortfalls stay rare.
