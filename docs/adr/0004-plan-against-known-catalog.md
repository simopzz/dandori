# Plan against a local Known Catalog, not live store lookups

The planner is grounded in a locally stored Known Catalog (pack sizes, availability, nutrition), seeded from the user's Esselunga order history and extended by occasional scrapes, rather than mapping Ingredients to products only at Cart Fill time. This lets the planner design Cook Sessions that use whole packs, especially of perishables, so rounding at Cart Fill is minimal. Price is cached but not load-bearing; sizes, availability and nutrition are. Cart Fill still verifies live availability and surfaces substitutions for approval.
