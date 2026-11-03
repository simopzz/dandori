# Users share nothing and choose their Language

Dandori becomes a hosted service for a few invited Users, each one person with their own kitchen. Nothing is shared between Users, not even the Known Catalog or Ingredients, and each User chooses one Language for both the pages and every food term in their kitchen. Sharing the Known Catalog or Ingredients would save scrapes and typing, but it would tie Users' food names, nutritional values and Languages together. A Language can be changed until the User's kitchen holds any food term; after that it stays, because an Ingredient is identified by its name and mixing languages splits one food in two. This supersedes ADR-0007.

## Consequences

- Every row belongs to one User, enforced by row-level security as well as by the queries.
- Sign-in is Supabase Auth, invite-only, by magic link.
- Changing Language once there is data would need a translation Run proposing every renamed food term: a later feature, not a setting.
