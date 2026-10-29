# Dandori

A personal meal-planning co-pilot for one person cooking for themselves. It plans batch-cooked meals, tracks what is in the kitchen, adapts to schedule changes, and fills an online supermarket cart.

## Language

### Planning

**Cycle**:
The stretch one Delivery has to last, from the first Slot after it arrives to a planned end date (typically about ten days later). Planning is done per Cycle to minimise delivery costs. A Cycle is upcoming from when it is opened until its Delivery arrives, current from then until the next Delivery arrives, and closed after that.
_Avoid_: Week, plan period

**Delivery**:
One supermarket delivery: its date and time window. It is tentative until the user marks it ordered, having checked out on the supermarket's site, which fixes the products it brings. It is taken to have arrived by the end of its window unless the user reports otherwise (late, moved to another day, cancelled, products missing or substituted).
_Avoid_: Order, shipment

**Plan**:
The Prep Sessions of a Cycle together with the Portion filling each of its Slots. There is one Plan per Cycle, and it changes only through an approved Plan Proposal.
_Avoid_: Schedule, meal plan

**Slot**:
A specific eating occasion within a Cycle (breakfast, lunch, dinner or snack on a given day, at that kind's time of day from the Profile), filled by a Portion or marked by an Exception.
_Avoid_: Meal, time block

**Plan Proposal**:
A set of changes to a Cycle's Plan, produced by the agent, that takes effect only when the user approves it. Opening a Cycle, Exceptions, Delivery reports and Cart Fill substitutions all lead to one.
_Avoid_: Recalculation, auto-update

**Exception**:
A logged deviation from what Dandori takes to happen: for a Slot, a Meal Out, skipped, or swapped (something else was eaten instead: another Portion or Stock Items); for a Prep Session, not prepared; for a Portion or Stock Item, discarded (expired, spoiled, lost). It records what happened, or, for a future Slot, what the user knows will happen (only a Meal Out or a skipped Slot can be logged ahead); when the Plan relied on it, the Plan Proposal that follows repairs the rest of the Plan.
_Avoid_: Event, interruption

**Meal Out**:
A meal not prepared from the user's kitchen (restaurant, takeaway, delivery), logged with estimated nutritional values.
_Avoid_: Eating out, craving, cheat meal

### Cooking and food

**Recipe**:
A reusable description of a dish: its Ingredients with their quantities, steps, required appliances, and how many Portions it makes as written.
_Avoid_: Dish

**Recipe Draft**:
A Recipe parsed from text the user pasted, or drafted by the planner for a Plan Proposal, awaiting their check: accepting it adds it to the Recipe Library, discarding it drops it. Approving a Plan Proposal accepts the Recipe Drafts it prepares, as drafted.
_Avoid_: Pending recipe, import

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
A food as such (chicken breast, rolled oats), measured in one unit (grams, millilitres or pieces), with its nutritional values per 100 g, per 100 ml or per piece; what a Recipe lists and what a Stock Item is a quantity of.
_Avoid_: Pantry item, product

**Kitchen Check**:
The user's confirmation, before an upcoming Cycle is planned, of the Portions and expiring Stock Items Dandori believes the kitchen holds, correcting what has drifted.
_Avoid_: Stocktake, inventory count

**Stock Item**:
A quantity of one Ingredient held in a Storage Location, with an optional expiry date.
_Avoid_: Inventory, pantry item

**Storage Location**:
Where a Portion or Stock Item is kept: pantry, fridge or freezer.
_Avoid_: Stash, shelf

**Content Language**:
The one language an installation of Dandori writes every food term in: Ingredient and Recipe names, steps, appliances, the Profile's exclusions and allergies, and what the agent says about food. Chosen once per installation and never changed.
_Avoid_: Locale, language preference, app language

### Goals

**Profile**:
The user's stored nutrition settings: the Targets, dietary exclusions and allergies, the default nutrition estimate for a Meal Out, the Untracked Allowance, and the time of day of each kind of Slot.
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
One purchasable supermarket item in the Known Catalog, under the supermarket's own name, with a pack size, nutritional values, its aisle (the supermarket's category for it), a cached price and last-seen availability.
_Avoid_: SKU, item

**Unlisted**:
Of a Catalog Product or a product in the user's order history: the supermarket currently sells nothing under its code, so it can be neither bought nor looked up. It may be listed again. Unlike an unavailable Catalog Product, which is listed but out of stock, an Unlisted one isn't on sale at all.
_Avoid_: Delisted, discontinued, gone

**Product Mapping**:
The user-editable link from an Ingredient to the Catalog Product preferred for buying it, which must be sold in the Ingredient's unit. One the agent proposes stays proposed until the user saves it, and the agent never replaces one that exists.
_Avoid_: Product match, SKU mapping

**Grocery List**:
What a Cycle's Plan needs minus current stock, rounded up to whole Catalog Products. Once its Delivery arrives, everything on it becomes Stock Items.
_Avoid_: Shopping list, order

**Cart Fill**:
Adding the Grocery List to the user's online supermarket cart, while its Delivery is still tentative. It stops at the cart: no checkout or payment.
_Avoid_: Order, purchase, checkout

### Agent

**Run**:
One execution of an agent task (planning, Recipe parsing, Known Catalog seed or refresh, Cart Fill), from its start until it succeeds, fails or is cancelled.
_Avoid_: Job, task, execution

**Trace**:
The ordered Trace Events of one Run, kept with the Run after it ends.
_Avoid_: Log, history

**Trace Event**:
One recorded step of a Run: a model message, a tool call or its result, a store action, a throttle wait, a retry or an error.
_Avoid_: Log entry, step

## Rules of the model

- A Prep Session at dinner yields three Portions by default: eaten now, eaten as lunch two days later, and a third scheduled two to three days after the second is eaten. The third is typically frozen. Other Prep Sessions yield as many Portions as their planned eating needs, often a single one eaten straight away.
- A Prep Session never yields more Portions than it has planned eating for. A Portion may still be without a Slot: when its planned eating falls after the Cycle ends, when it is carried over from a previous one, or while an Exception awaits a Plan Proposal. The planner uses Portions without a Slot before scheduling new Prep Sessions.
- A Prep Session of a Recipe scales its Ingredients to the Portions the Prep Session yields: a Recipe that makes four Portions, cooked for three, uses three quarters of each Ingredient. Nutritional values per Portion are the same either way.
- Each food is one Ingredient: plurals and size or grade words (eggs, large egg, extra virgin olive oil) don't make a new one; only a nutritionally different food does (egg white is not egg). A Recipe reuses existing Ingredients and adds a new one only once the user confirms its nutritional values.
- Every food term is in the Content Language, whatever language it arrived in: a Recipe pasted in another language is translated when it is parsed. Ingredients get generic food names ("petto di pollo"); product names belong only to Catalog Products.
- Portions and Stock Items can each live in any Storage Location; a frozen Portion is just a Portion in the freezer. A Portion's location depends on what it is and how long it must keep.
- Targets are evaluated over a trailing seven days, independent of Cycle boundaries: calories within ±5%, protein as a floor. A changed Target applies from the first Slot of the next Plan Proposal approved after it; each earlier day keeps the Target it was planned with, and an approved Plan is never rechecked.
- An Exception logged ahead is input to the planning Run when the Plan doesn't rely on it yet, and leads to a Plan Proposal like any other when it does. Its Slot gets no Portion and no Prep Session; when cooking is impossible on a day without one, the user says so in a note. A Meal Out counts toward the Targets with the Profile's default estimate unless the Exception gives its own; a skipped Slot counts as nothing. Changing or removing it works the same way. It belongs to its day and Slot kind, not to a Cycle: it outlives a moved Delivery or a dropped Cycle, and counts toward the Targets even if its day ends up between Cycles. Once its day is past it stands as what happened, unless the user changes it.
- A Stock Item past its expiry date stays in the kitchen until the user logs it discarded, but no Plan uses it.
- The Plan is taken to happen as dated until an Exception says otherwise: a Prep Session is prepared from its day on, using up the Stock Items of its Ingredients, and a Portion is eaten once the day of its Slot is past. Prepared Portions not yet eaten, and Stock Items, are what the kitchen holds.
- At most one Plan Proposal is pending at a time, across all Cycles. Whatever would lead to a new one (an Exception, a Delivery report, a Cart Fill substitution) rejects the pending one with a note saying why, and the next planning Run takes both into account.
- A Plan Proposal replaces the Plan only from a Slot on, never one already past: the Prep Sessions before it stay, and so do today's, already taken as prepared, and their Portions not yet eaten are the Plan Proposal's to put in Slots again.
- A Cycle's end date moves through an approved Plan Proposal, typically extending it when Exceptions leave enough Portions and stock to last longer, and with its Delivery: a Delivery moved later moves the end date as many days later, since it is to last as long. One moved earlier leaves the end date where it was, for the next Plan Proposal to judge from what is left.
- Cycles never overlap, and at most one is upcoming. A Slot belongs to the upcoming Cycle when it comes after the end of that Cycle's Delivery window, so Slots earlier on the delivery day stay with the current Cycle.
- A Delivery arriving before the current Cycle's end date closes that Cycle early, and what is left carries over as Portions without a Slot and Stock Items. Its Plan Proposals still pending are rejected, since its Plan no longer changes.
- An upcoming Cycle's Delivery can be moved until it arrives. A move, or a Plan Proposal approved in the current Cycle that changes what will carry over, leaves the upcoming Cycle's Plan needing a new Plan Proposal; it never changes silently. Once its Delivery is ordered, that Plan Proposal must make do with what is ordered and what carries over.
- An upcoming Cycle can be cancelled, by dropping it while its Delivery is tentative or by reporting the Delivery cancelled once ordered. It is dropped with its Plan Proposals, and its days become days between Cycles.
- Cycles may have days between them. Those days have no Plan and no Slots, but what is eaten on them still counts toward the Targets.
- Cancelling a Run rolls nothing back: what it already did (products in the cart, Catalog Products seeded) stays, and running it again finishes the work. A cancelled planning Run produces no Plan Proposal.
- A Run still going when Dandori stops counts as failed, not cancelled: only the user cancels a Run.
