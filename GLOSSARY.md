# Dandori

A personal meal-planning co-pilot for one person cooking for themselves. It plans batch-cooked meals, tracks what is in the kitchen, adapts to schedule changes, and fills an online supermarket cart.

## Language

### Planning

**Cycle**:
The stretch one Delivery has to last, from the first Slot after it arrives to a planned end date (typically about ten days later). Planning is done per Cycle to minimise delivery costs. A Cycle is upcoming from when it is opened until its Delivery arrives, current from then until the next Delivery arrives, and closed after that.
_Avoid_: Week, plan period

**Delivery**:
One supermarket delivery: its date and time window. It is tentative until the user marks it ordered, having checked out on the supermarket's site, which fixes the products it brings. It is taken to have arrived by the end of its window, ordered or not, unless the user reports otherwise (late, moved to another day, cancelled, products missing or substituted); once arrived it is neither tentative nor ordered, and one never marked ordered says so.
_Avoid_: Order, shipment

**Plan**:
The Prep Sessions of a Cycle together with the Portion filling each of its Slots. There is one Plan per Cycle, and it changes only through an approved Plan Proposal.
_Avoid_: Schedule, meal plan

**Slot**:
A specific eating occasion within a Cycle (breakfast, lunch, dinner or snack on a given day, at that kind's time of day from the Profile), filled by a Portion or marked by an Exception.
_Avoid_: Meal, time block

**Plan Proposal**:
A set of changes to a Cycle's Plan, produced by the agent, that takes effect only when the user approves it. Opening a Cycle, Exceptions, Delivery reports, Cart Fill substitutions and the user asking for changes all lead to one.
_Avoid_: Recalculation, auto-update

**Replaced**:
A pending Plan Proposal rejected by something that happened rather than by the user: an Exception, a Delivery report, a Cart Fill substitution or a moved Delivery. It names what replaced it ("replaced by the Exception on Fri 6 Nov, dinner"), which is never a Planning Note.
_Avoid_: Superseded, auto-rejected

**Planning Note**:
What the user tells the planner about one Cycle, when rejecting a Plan Proposal or asking for changes to the Plan: any feedback, from a complaint to a request, several at once if they like. It belongs to its Cycle.
_Avoid_: Rejection Note, revision, feedback item

**Planning Brief**:
The user's lasting wishes for how their meals are planned (variety, effort, what they enjoy), as free text every planning Run reads. Unlike the Profile, it is guidance the planner follows only as far as the rules of the Plan allow.
_Avoid_: Preferences, settings

**Exception**:
A logged deviation from what Dandori takes to happen: for a Slot, a Meal Out, skipped, or swapped (something else was eaten instead: another Portion or Stock Items); for a Prep Session, not prepared; for a Portion, discarded (spoiled, lost). Stock Items have none: spoiled stock is corrected on the Stock Item, and what that leaves short is a Shortfall. It records what happened, or, for a future Slot, what the user knows will happen (only a Meal Out or a skipped Slot can be logged ahead); when the Plan relied on it, the Plan Proposal that follows repairs the rest of the Plan.
_Avoid_: Event, interruption

**Meal Out**:
A meal not prepared from the user's kitchen (restaurant, takeaway, delivery), logged with estimated nutritional values.
_Avoid_: Eating out, craving, cheat meal

### Cooking and food

**Recipe**:
A reusable description of a dish: its Ingredients with their quantities, steps, required appliances, how many Portions it makes as written, and the kinds of Slot its Portions may be eaten at (a breakfast Recipe may also fill a snack, never a lunch).
_Avoid_: Dish

**Recipe Draft**:
A Recipe parsed from text the user pasted, or drafted by the planner for a Plan Proposal, awaiting their check: accepting it adds it to the Recipe Library, discarding it drops it. Approving a Plan Proposal accepts the Recipe Drafts it prepares, as drafted.
_Avoid_: Pending recipe, import

**Recipe Library**:
The persisted collection of every Recipe the user has accepted, whether imported or agent-generated.
_Avoid_: Cookbook, recipe database

**Prep Session**:
One preparation of food that yields one or more Portions: cooking or assembling a single Recipe, or setting out one Ingredient eaten as it is, in which case it has no Recipe. Anything that combines Ingredients is a Recipe, however simple.
_Avoid_: Cook session, batch, meal prep, direct meal

**Portion**:
One ready-to-eat serving produced by a Prep Session, kept in a Storage Location and assigned to at most one Slot.
_Avoid_: Serving, leftover

**Ingredient**:
A food as such (chicken breast, rolled oats), measured in one unit (grams, millilitres or pieces), with its nutritional values per 100 g, per 100 ml or per piece; what a Recipe lists and what a Stock Item is a quantity of. It is measured as it goes into a dish: drained, for food packed in liquid.
_Avoid_: Pantry item, product

**Kitchen Ledger**:
The record of every Movement in the kitchen, in the order they happen; what the kitchen holds at any moment is what it adds up to then.
_Avoid_: Inventory log, history

**Movement**:
One change to what the kitchen holds, with when it happened and what caused it: Stock Items seeded, delivered, used up by a Prep Session, corrected or discarded; Portions prepared, moved between Storage Locations, eaten or discarded. It is assumed when Dandori takes it to have happened as planned, and reported when the user says what happened (a Count, an Exception, a Delivery report); nothing assumed needs the user's confirmation. An assumed Movement is recorded once, when its time passes, as the Plan and its Recipes were then: changing them later never changes it.
_Avoid_: Transaction, delta, stock change

**Today**:
What Dandori shows the user from the moment they open it: the whole day's Slots, the next one first and the later ones a click away; when to start preparing and with what; what to take out of the freezer tonight; a Delivery due; the next Cycle to plan and order, from three days before its Delivery; and what it has assumed since they last looked, for them to correct if it went otherwise.
_Avoid_: Dashboard, home

**Kitchen Check**:
The user's confirmation, before an upcoming Cycle is planned, of every Portion and Stock Item Dandori believes the kitchen holds today, correcting what has drifted with Counts. Skipping it counts as confirming.
_Avoid_: Stocktake, inventory count

**Count**:
A correction the user makes by saying how much of a Stock Item, or how many Portions, the kitchen actually holds: a Movement of the difference from what Dandori believed, which keeps both.
_Avoid_: Adjustment, delta entry

**Staple**:
An Ingredient the user marks as always in the kitchen and too fiddly to weigh, such as olive oil, salt or spices. It counts toward nutrition like any other, but no Stock Item holds it and no Grocery List buys it: the Grocery List reminds the user to check there is still some.
_Avoid_: Pantry basic, condiment

**Stock Item**:
A quantity of one Ingredient held in a Storage Location. It has no expiry date: the user watches for spoiled food and corrects the Stock Item.
_Avoid_: Inventory, pantry item

**Shortfall**:
What a Prep Session of the current Cycle needs of an Ingredient beyond the Stock Items the kitchen will hold on its day, once the Prep Sessions before it have used theirs up, when its Plan counted on them being there: something unexpected took them away (spoiled food, a correction, a short Delivery). Dandori finds it, nobody logs it: it follows stock and the Plan, and goes once the stock is there. An upcoming Cycle has none: what its Plan needs beyond the stock is what its Grocery List buys. It is not an Exception, and never leads to a Plan Proposal unless the user asks for one.
_Avoid_: Missing stock, deficit, stock Exception

**Storage Location**:
Where a Portion or Stock Item is kept: pantry, fridge or freezer.
_Avoid_: Stash, shelf

**Language**:
The one language a User's Dandori speaks: its pages, and every food term in their kitchen (Ingredient and Recipe names, steps, appliances, the Profile's exclusions and allergies, and what the agent says about food). Chosen by the User, who can change it until their kitchen holds any food term; after that it stays.
_Avoid_: Content Language, locale, app language

**User**:
One person cooking for themselves, with their own kitchen: everything in Dandori (Profile, Recipes, Ingredients, Known Catalog, Cycles, Kitchen Ledger) belongs to exactly one User, and nothing is shared between Users.
_Avoid_: Account, tenant, household

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
One purchasable supermarket item in the Known Catalog, under the supermarket's own name, with a pack size (and, for food packed in liquid, what a pack holds drained), nutritional values, its aisle (the supermarket's category for it), a cached price and last-seen availability.
_Avoid_: SKU, item

**Unlisted**:
Of a Catalog Product or a product in the user's order history: the supermarket currently sells nothing under its code, so it can be neither bought nor looked up. It may be listed again. Unlike an unavailable Catalog Product, which is listed but out of stock, an Unlisted one isn't on sale at all.
_Avoid_: Delisted, discontinued, gone

**Product Mapping**:
The user-editable link from an Ingredient to the Catalog Product preferred for buying it, which must be sold in the Ingredient's unit. One the agent proposes stays proposed until the user saves it, and the agent never replaces one that exists.
_Avoid_: Product match, SKU mapping

**Grocery List**:
What a Cycle's Plan needs minus the stock the kitchen will hold when its Delivery arrives (the Stock Items left once the current Cycle's Prep Sessions have used theirs up), rounded up to whole Catalog Products through each Ingredient's Product Mapping, grouped by aisle. An Ingredient with no Product Mapping is flagged and listed at what is needed. It follows the Plan until its Delivery is marked ordered, which fixes it; so a Delivery is marked ordered only once its Cycle has a Plan. Once its Delivery arrives, everything on it becomes Stock Items, the rounding surplus included, each in the Storage Location its aisle suggests.
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
One recorded step of a Run: a model message, a tool call or its result, a store action, a throttle wait, a retry, the tokens a model turn used or an error.
_Avoid_: Log entry, step

## Rules of the model

- A Prep Session at dinner yields three Portions by default: eaten now, eaten as lunch two days later, and a third scheduled two to three days after the second is eaten. The third is typically frozen. Other Prep Sessions yield as many Portions as their planned eating needs, often a single one eaten straight away.
- A Prep Session never yields more Portions than it has planned eating for. A Portion may still be without a Slot: when its planned eating falls after the Cycle ends, when it is carried over from a previous one, or while an Exception awaits a Plan Proposal. The planner uses Portions without a Slot before scheduling new Prep Sessions.
- A Prep Session of a Recipe scales its Ingredients to the Portions the Prep Session yields: a Recipe that makes four Portions, cooked for three, uses three quarters of each Ingredient. Nutritional values per Portion are the same either way.
- Each food is one Ingredient: plurals and size or grade words (eggs, large egg, extra virgin olive oil) don't make a new one; only a nutritionally different food does (egg white is not egg), or one the kitchen can't make from the other, which the Grocery List must buy as such (minced chicken is not chicken breast). A Recipe reuses existing Ingredients and adds a new one only once the user confirms its nutritional values: on its Recipe Draft, where they can be edited, or by approving a Plan Proposal that prepares the draft as it is.
- Every food term is in the User's Language, whatever language it arrived in: a Recipe pasted in another language is translated when it is parsed. Ingredients get generic food names ("petto di pollo"); product names belong only to Catalog Products.
- Portions and Stock Items can each live in any Storage Location; a frozen Portion is just a Portion in the freezer. A Portion's location depends on what it is and how long it must keep.
- Targets are evaluated over a trailing seven days, independent of Cycle boundaries: calories within ±5%, protein as a floor. A changed Target applies from the first Slot of the next Plan Proposal approved after it; each earlier day keeps the Target it was planned with, and an approved Plan is never rechecked.
- An Exception logged ahead is input to the planning Run when the Plan doesn't rely on it yet, and leads to a Plan Proposal like any other when it does. Its Slot gets no Portion and no Prep Session; when cooking is impossible on a day without one, the user says so in a note. A Meal Out counts toward the Targets with the Profile's default estimate unless the Exception gives its own; a skipped Slot counts as nothing. Changing or removing it works the same way. It belongs to its day and Slot kind, not to a Cycle: it outlives a moved Delivery or a dropped Cycle, and counts toward the Targets even if its day ends up between Cycles. Once its day is past it stands as what happened, unless the user changes it.
- A Plan uses first the Stock Items that would spoil first, judging by what they are and their Storage Location (fresh fish in the fridge before rice in the pantry). Dandori records no expiry dates.
- A Prep Session uses up its Ingredients once, from the Stock Items holding each, the oldest first, whatever their Storage Location. A Stock Item used up leaves the kitchen. Stock that falls short is used up as far as it goes, never below nothing. A correction the user makes to a Stock Item afterwards stands: it is never used up again. A Prep Session logged not prepared gives back what it used up, as it was.
- The Plan is taken to happen as dated until an Exception says otherwise: a Prep Session is prepared from its Slot's time of day on, using up the Stock Items of its Ingredients, and a Portion is eaten once its Slot's time of day is past. Until then they are planned; after, assumed, until the user confirms or corrects them. Prepared Portions not yet eaten, and Stock Items, are what the kitchen holds.
- What the kitchen holds is never stored as such: it is what the Kitchen Ledger adds up to at a given moment. Asking why a Stock Item holds what it does is answered by its Movements.
- At most one Plan Proposal is pending, or one planning Run going, at a time, across all Cycles. Asking to plan waits its turn: it is refused while either is, and an upcoming Cycle is planned only once the current Cycle's pending Plan Proposal is decided. Something that happened replaces instead: an Exception on the current Cycle, or a moved Delivery, replaces the pending Plan Proposal and cancels the planning Run going on either Cycle; an Exception on the upcoming Cycle does so only for the upcoming Cycle; one between Cycles, for neither. The next planning Run reads what happened for itself.
- A Plan Proposal approved in the current Cycle changes what will carry over when it changes how many Portions of each Recipe the upcoming Delivery would carry over.
- Rejecting a Plan Proposal with a Planning Note asks for another: a new planning Run starts from the rejected one's first Slot, or from the first Slot not yet prepared once that is past, whether or not the Cycle has a Plan. Without a note the rejection asks for nothing, and the user can start planning again whenever an Exception awaits a Plan Proposal and none is pending. Every planning Run of a Cycle is told all its Planning Notes, newest first, and a newer one wins where they disagree; they never reach another Cycle, where lasting wishes belong in the Planning Brief.
- The user can ask for changes to an upcoming or current Cycle's Plan at any time, with a Planning Note, while no Plan Proposal is pending and no planning Run is going: a planning Run starts from the first Slot not yet prepared, and the Plan stays as it is until its Plan Proposal is approved.
- The Planning Brief is written only by the user. Changing it never starts a planning Run nor leaves a Plan needing a new Plan Proposal: it applies from the next planning Run. Where a Planning Note disagrees with it, the note wins within its Cycle.
- A Plan Proposal replaces the Plan only from a Slot on whose time of day is still to come: the Prep Sessions before it stay, and their Portions not yet eaten are the Plan Proposal's to put in Slots again.
- An Exception on a Slot whose time is already past is logged after the fact: it corrects the Movements Dandori assumed for it, so a Portion assumed eaten is held again.
- Frozen food needed for a Slot or a Prep Session is thawed the evening before, after the previous day's dinner: Dandori plans its move to the fridge then and reminds the user from that time on. The planner uses frozen food only where such an evening is still to come.
- A Cycle's end date moves through an approved Plan Proposal, typically extending it when Exceptions leave enough Portions and stock to last longer, and with its Delivery: a Delivery moved later moves the end date as many days later, since it is to last as long. One moved earlier leaves the end date where it was, for the next Plan Proposal to judge from what is left.
- Cycles never overlap, and at most one is upcoming. A Slot belongs to the upcoming Cycle when it comes after the end of that Cycle's Delivery window, so Slots earlier on the delivery day stay with the current Cycle.
- A Delivery arriving before the current Cycle's end date closes that Cycle early, and what is left carries over as Portions without a Slot and Stock Items. Its Plan Proposals still pending are rejected, and its planning Run going is cancelled, since its Plan no longer changes: they were not replaced, their Cycle closed, and the Run names the Delivery that closed it.
- An upcoming Cycle's Delivery can be moved until it arrives. A move, or a Plan Proposal approved in the current Cycle that changes what will carry over, leaves the upcoming Cycle's Plan needing a new Plan Proposal; it never changes silently. Once its Delivery is ordered, that Plan Proposal must make do with what is ordered and what carries over.
- An upcoming Cycle can be cancelled, by dropping it while its Delivery is tentative or by reporting the Delivery cancelled once ordered. It is dropped with its Plan Proposals, and its planning Run going is cancelled, since its Cycle was dropped; the current Cycle's pending Plan Proposal stays. Its days become days between Cycles.
- Cycles may have days between them. Those days have no Plan and no Slots, but what is eaten on them still counts toward the Targets.
- Cancelling a Run rolls nothing back: what it already did (products in the cart, Catalog Products seeded) stays, and running it again finishes the work. A cancelled planning Run produces no Plan Proposal.
- A Run still going when Dandori stops counts as failed, not cancelled: only the user cancels a Run, or, for a planning Run, whatever would replace its Plan Proposal or ends its Cycle (a Delivery closing it, or the Cycle dropped), and the Run names it.
