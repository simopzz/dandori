# MealOps

A personal meal-planning co-pilot for one person cooking for themselves. It plans batch-cooked meals, tracks what is in the kitchen, adapts to schedule changes, and fills an online supermarket cart.

## Language

### Planning

**Cycle**:
The stretch one grocery delivery has to last, from the day it arrives to a planned end date (typically about ten days later). Planning is done per Cycle to minimise delivery costs.
_Avoid_: Week, plan period

**Plan**:
The Prep Sessions of a Cycle together with the Portion filling each of its Slots. There is one Plan per Cycle, and it changes only through an approved Plan Proposal.
_Avoid_: Schedule, meal plan

**Slot**:
A specific eating occasion within a Cycle (breakfast, lunch, dinner or snack on a given day), filled by a Portion or marked by an Exception.
_Avoid_: Meal, time block

**Plan Proposal**:
A set of changes to the Plan, produced by the agent, that takes effect only when the user approves it. Exceptions and Cart Fill substitutions both lead to one.
_Avoid_: Recalculation, auto-update

**Exception**:
A logged deviation from the Plan for a Slot: a Meal Out, skipped, or swapped (something else was eaten instead: another Portion or Stock Items). It records what actually happened; the Plan Proposal that follows repairs the rest of the Plan.
_Avoid_: Event, interruption

**Meal Out**:
A meal not prepared from the user's kitchen (restaurant, takeaway, delivery), logged with estimated nutritional values.
_Avoid_: Eating out, craving, cheat meal

### Cooking and food

**Recipe**:
A reusable description of a dish, its Ingredients, steps and required appliances.
_Avoid_: Dish

**Recipe Library**:
The persisted collection of every Recipe the user has accepted, whether imported or agent-generated.
_Avoid_: Cookbook, recipe database

**Prep Session**:
One preparation of food that yields one or more Portions: cooking or assembling a single Recipe, or setting out Ingredients eaten as they are, in which case it has no Recipe.
_Avoid_: Cook session, batch, meal prep, direct meal

**Portion**:
One ready-to-eat serving produced by a Prep Session, kept in a Storage Location and assigned to at most one Slot.
_Avoid_: Serving, leftover

**Ingredient**:
A food as such (chicken breast, rolled oats), with its nutritional values; what a Recipe lists and what a Stock Item is a quantity of.
_Avoid_: Pantry item, product

**Stock Item**:
A quantity of one Ingredient held in a Storage Location, with an optional expiry date.
_Avoid_: Inventory, pantry item

**Storage Location**:
Where a Portion or Stock Item is kept: pantry, fridge or freezer.
_Avoid_: Stash, shelf

### Goals

**Profile**:
The user's stored nutrition settings: the Targets, dietary exclusions and allergies, the default nutrition estimate for a Meal Out, and the Untracked Allowance.
_Avoid_: Settings, preferences

**Target**:
A daily calorie or macro goal held in the Profile.
_Avoid_: Goal, budget

**Untracked Allowance**:
A fixed daily amount of calories and macros the user expects to eat outside the Plan, reserved before Slots are filled.
_Avoid_: Buffer, fudge factor

**Target Proposal**:
A suggested change to the Targets, produced by the agent, that takes effect only when the user approves it.
_Avoid_: Auto-adjustment

**Weight Check-in**:
A recorded body-weight measurement that the agent uses to make Target Proposals.
_Avoid_: Weigh-in, log

### Groceries

**Known Catalog**:
The locally stored subset of the supermarket's products the planner knows about: pack sizes, availability and nutritional values, seeded from the user's order history and extended by occasional catalog scrapes.
_Avoid_: Product database, store cache

**Catalog Product**:
One purchasable supermarket item in the Known Catalog, with a pack size, nutritional values and last-seen availability.
_Avoid_: SKU, item

**Product Mapping**:
The user-editable link from an Ingredient to the Catalog Product preferred for buying it.
_Avoid_: Product match, SKU mapping

**Grocery List**:
What a Cycle's Plan needs minus current stock, rounded up to whole Catalog Products. Any surplus from rounding becomes Stock Items.
_Avoid_: Shopping list, order

**Cart Fill**:
Adding the Grocery List to the user's online supermarket cart. It stops at the cart: no checkout or payment.
_Avoid_: Order, purchase, checkout

## Rules of the model

- A Prep Session at dinner yields three Portions by default: eaten now, eaten as lunch two days later, and a third scheduled two to three days after the second is eaten. The third is typically frozen. Other Prep Sessions yield as many Portions as their planned eating needs, often a single one eaten straight away.
- A Prep Session never yields more Portions than it has planned eating for. A Portion may still be without a Slot: when its planned eating falls after the Cycle ends, when it is carried over from a previous one, or while an Exception awaits a Plan Proposal. The planner uses Portions without a Slot before scheduling new Prep Sessions.
- Portions and Stock Items can each live in any Storage Location; a frozen Portion is just a Portion in the freezer. A Portion's location depends on what it is and how long it must keep.
- Targets are evaluated over a trailing seven days, independent of Cycle boundaries: calories within ±5%, protein as a floor.
- A Cycle's end date moves only through an approved Plan Proposal, typically extending it when Exceptions leave enough Portions and stock to last longer.
- Cycles never overlap: a delivery arriving before the current Cycle's end date closes that Cycle early, and what is left carries over as Portions without a Slot and Stock Items.
- Cycles may have days between them. Those days have no Plan and no Slots, but what is eaten on them still counts toward the Targets.
