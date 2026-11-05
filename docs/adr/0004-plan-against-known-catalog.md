# Plan against a local Known Catalog, not live store lookups

The planner is grounded in a locally stored Known Catalog (pack sizes, availability, nutrition), seeded from the user's Esselunga order history and extended by occasional scrapes, rather than mapping Ingredients to products only at Cart Fill time. This lets the planner design Prep Sessions that use whole packs, especially of perishables, so rounding at Cart Fill is minimal. Price is cached but not load-bearing; sizes, availability and nutrition are. Cart Fill still verifies live availability and surfaces substitutions for approval.

## Consequences

- v2's first milestone plans without a Known Catalog: seeding one needs the browser extension (ADR-0016), which comes after it. Until then every Ingredient is without a Product Mapping, the Grocery List lists what is needed in each Ingredient's unit, and the User fills the cart by hand. The Known Catalog is the first step after it.
