# Appliances are a fixed list of kinds, not Food Terms

A Recipe needing an appliance the User doesn't own can't be in a Plan, a hard rule (ADR-0018), so a Recipe's appliances must match the Profile's exactly. As free Food Terms they matched only by spelling: a Recipe parsed with "piano cottura" would never meet a Profile owning "fornelli". So appliances are a fixed list of kinds (oven, hob, microwave, air fryer, pressure cooker, slow cooker, blender, food processor, stand mixer, grill, toaster, steamer), translated like the pages rather than written in the User's Language. A Recipe lists the kinds it needs, chosen by the parser from the list, and the Profile the kinds the User owns, each with a free-text note on its model ("Instant Pot Duo 6 L") that the planner reads and nothing compares.

## Considered Options

- **Free text, the Profile as the vocabulary**: the Recipe form picks from the User's own appliance names and the parser is told to use them. It names a model directly, but renaming an appliance in the Profile orphans every Recipe that needs it.
- **A list of other equipment beside the kinds**, for needs the list lacks: it would repeat what the steps already say, and still check nothing.

## Consequences

- An appliance is not a Food Term, and neither is a model note: writing one doesn't make the Language stay.
- Equipment outside the list (an ice-cream maker) lives only in a Recipe's steps, unchecked: a Recipe needing it can be planned for a User without one. Adding a kind is a code change.
- A new User owns a hob and an oven until they say otherwise.
