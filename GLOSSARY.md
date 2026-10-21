# MealOps

A personal meal-planning co-pilot for one person cooking for themselves. It plans batch-cooked meals, tracks what is in the kitchen, adapts to schedule changes, and fills an online supermarket cart.

## Language

### Planning

**Cycle**:
The stretch of roughly ten days one grocery delivery has to last, beginning the day the delivery arrives. Planning is done per Cycle to minimise delivery costs.
_Avoid_: Week, plan period

**Slot**:
A specific eating occasion within a Cycle (breakfast, lunch, dinner or snack on a given day) that a Portion or an exception is assigned to.
_Avoid_: Meal, time block

**Plan Proposal**:
A set of changes to the current plan, produced by the agent, that takes effect only when the user approves it.
_Avoid_: Recalculation, auto-update

**Exception**:
A logged deviation from the plan for a Slot: eaten out, skipped, swapped, craving, or a last-minute change. It is the trigger for a Plan Proposal.
_Avoid_: Event, interruption

### Cooking and food

**Recipe**:
A reusable description of a dish, its ingredients, steps and required appliances.
_Avoid_: Dish

**Recipe Library**:
The persisted collection of every Recipe the user has accepted, whether imported or agent-generated.
_Avoid_: Cookbook, recipe database

**Cook Session**:
One cooking event for a single Recipe that yields several Portions (default three).
_Avoid_: Batch, meal prep

**Portion**:
One ready-to-eat serving produced by a Cook Session, assigned to exactly one Slot and kept in a Storage Location.
_Avoid_: Serving, leftover

**Ingredient**:
A raw or packaged food item bought to be cooked with, tracked as stock with a quantity and an optional expiry date.
_Avoid_: Pantry item, product

**Storage Location**:
Where a Portion or Ingredient is kept: pantry, fridge or freezer. A Portion's location depends on the Recipe and its keeping time.
_Avoid_: Stash, shelf

### Goals

**Profile**:
The user's stored nutrition settings: calorie and macro Targets, dietary exclusions and allergies, and the default estimate used for a meal out.
_Avoid_: Settings, preferences

**Target**:
The daily calorie and macro goal derived from the Profile, evaluated over a trailing seven days (calories within ±5%, protein as a floor).
_Avoid_: Goal, budget

**Weight Check-in**:
A recorded body-weight measurement that the agent uses to propose adjustments to the Targets.
_Avoid_: Weigh-in, log

### Groceries

**Known Catalog**:
The locally stored subset of the supermarket's products the planner knows about: pack sizes, availability and nutritional values, seeded from the user's order history and extended by occasional catalog scrapes. Planning is grounded in it so pack sizes are used fully, especially for perishables.
_Avoid_: Product database, store cache

**Catalog Product**:
One purchasable supermarket item in the Known Catalog, with a pack size, nutritional values and last-seen availability.
_Avoid_: SKU, item

**Cart Fill**:
Adding the products needed for a Cycle to the user's online supermarket cart. It stops at the cart: no checkout or payment.
_Avoid_: Order, purchase, checkout

## Rules of the model

- A Cook Session yields three Portions by default: eaten now, eaten as lunch two days later, and a third scheduled two to three days after the second is eaten. The third is typically frozen.
- Every Portion belongs to a Slot; nothing is stored without a planned Slot.
- Only lunch and dinner Slots are filled by Cook Sessions; breakfast and snack Slots are filled directly by a Recipe or Ingredient.
- Portions and Ingredients can each live in any Storage Location, and a frozen Portion is just a Portion in the freezer.
