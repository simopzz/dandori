# Vision

> v1's original pitch is kept on `main`. Where this differs from `GLOSSARY.md` or `docs/adr/`, those win.

Dandori is a meal-planning co-pilot for one person cooking for themselves, who wants to eat structured, healthy meals without planning and cooking becoming a second job. It plans batch-cooked meals around one supermarket Delivery at a time, knows what is in the kitchen and when, tells you what to do today, and adapts when the week doesn't go to plan. A few invited Users each have their own Dandori: their own kitchen, Recipes and Language, sharing nothing.

## Ideas

- **Cook once, eat three times.** A dinner yields three Portions: one now, one for lunch two days later, one for the freezer.
- **Low-care cooking.** Recipes where the oven or the Instant Pot does the work: a few active minutes, the rest unattended.
- **Plan per Delivery.** A Cycle is what one Delivery has to last, about ten days, to keep delivery costs down.
- **The agent proposes, you approve.** The Plan changes only through a Plan Proposal you approve, and you see every step the agent takes.
- **What wasn't contradicted happened.** Dandori assumes the Plan went as planned at each Slot's time. You correct only what went otherwise.
- **Targets are aimed at, not enforced.** A Plan aims at your calories and protein over the days it plans, and shows where it lands. It's your call to approve one that misses.
- **Nothing is bought for you.** Dandori never checks out.

## A Cycle, week to week

1. **Three days before the next Delivery**, Today reminds you to plan the next Cycle. The planner reads your Profile, Planning Brief, the Recipe Library, what the kitchen will hold when the Delivery arrives and the Exceptions you've logged ahead, and proposes a Plan: Prep Sessions, which Portion fills each Slot, and what goes in the freezer.
2. **You approve it**, or reject it with a Planning Note saying what to change, and it plans again.
3. **You order.** The Grocery List is what the Plan needs beyond what the kitchen will hold, with Staples to check. You fill the supermarket cart and mark the Delivery ordered.
4. **The Delivery arrives.** Today tells you where everything goes: most of it in its usual place, some of a fresh Ingredient used late in the Cycle in the freezer.
5. **Every day**, Today shows the day's Slots, the next one first: what to eat, when to start preparing and with what, and what to take out of the freezer tonight. It lists what it assumed since you last looked, so you can correct a snack you forgot or a dinner you skipped.
6. **When the week changes**, you log an Exception: a Meal Out, a skipped Slot, a swap, a Prep Session not prepared, a Portion thrown away. If the Plan relied on it, a Plan Proposal repairs the rest.
7. **What carries over** (Portions in the freezer, stock left) is what the next Cycle's Plan starts from.

## Features

**M1: one real Cycle on v2, v1 retired**

- Ingredients with their nutrition (calories, protein, carbs, fat, fibre, sugar), usual Storage Location and Staples. The Kitchen Ledger, and Counts to correct it.
- Recipes pasted and parsed by the agent into Recipe Drafts you check, with active and total minutes and the kinds of Slot they suit. Every Run with its live Trace.
- The Profile: Targets, exclusions and allergies, the appliances you own, Slot times, the Meal Out estimate, and the Planning Brief.
- Cycles, Deliveries, the planner and Plan Proposals, planning the next Cycle against what carries over.
- The Grocery List, filled into the cart by hand.
- The Plan over time on Today: preparing, thawing, putting away, and what was assumed.
- Exceptions, and the Plan Proposals that repair around them.

**M2**

- The Known Catalog and Product Mapping, read from the supermarket through a browser extension, and Cart Fill through the same extension, stopping at the cart.
- The Kitchen Check before planning, Shortfalls, Delivery reports, Plan Proposals that move a Cycle's end date, and Ingredients linked to your exclusions and allergies.

**Later**

- Browsing earlier Cycles, Weight Check-ins and Target Proposals, a guided profiler for Targets, Cooking Mode with the screen kept on and timers, and searching Recipes by meaning.
