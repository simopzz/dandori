# Plans aim at Targets; only hard rules stop them

A Plan Proposal is stopped only by a hard rule, one whose breach leaves a Plan that makes no sense or isn't safe: a Slot filled twice or left empty, a Portion eaten before it is prepared or that doesn't exist, food outside the Cycle or on a Slot with an Exception, an allergen or exclusion, a Recipe at a kind of Slot it isn't for, an appliance the User doesn't own. Everything else is soft: the Targets, Portions eaten back to back, a kept Portion left unused. A soft miss never fails a planning Run. It is reported to the planner so it can do better, and shown on the Plan Proposal for the User to weigh.

The Targets are aimed at on average over the days the Plan Proposal plans, Meal Outs on those days included, with no planned day more than 25% off. Days already eaten are shown but never made up. In v1 every rule blocked, over every trailing seven days, and the planner did the sums in its head: on 1 Nov 2026 a planning Run swung between too little protein and too many calories, then failed after three attempts at 1,116 g of protein against 1,120 g over a week, leaving the User with no Plan at all. The planner now has a `check_plan` tool where Dandori does the sums, returning each day's nutrition, the Targets' results and every violation without storing anything. `submit_plan` stores the first Plan that keeps every hard rule, and a Run fails only after three Plans that break one.

## Considered Options

- **v1's gate, loosened tolerances**: a wider band still kills a Run at its edge, and still asks the LLM to do the arithmetic.
- **Trailing seven days, soft**: makes the planned days make up for days already eaten, a stricter weekly budget that is much harder to hit with whole Portions. The trailing seven days stay on the Plan page, for information.
- **A solver sizing Portions**: hits the numbers exactly, but Dandori would decide what is cooked, and Portions would come in any size. Instead the planner chooses each Prep Session's Portion size, between 0.75× and 1.5× of the Recipe as written, and `check_plan` shows what that does: a "serves 4" Recipe at 450 kcal can then fill a 700 kcal dinner, which whole Portions at the written size never could.

## Consequences

- Carbs and fat have no Target: they are shown, never aimed at. Calories are a range, protein and fibre floors, sugar a ceiling.
- An approved Plan can miss a Target. That is the User's call, made with the miss in front of them.
