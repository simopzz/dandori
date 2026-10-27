# One Content Language per installation

Every food term Dandori stores or the agent writes is in a single Content Language, Italian or English. The language is set per installation (`DANDORI_CONTENT_LANGUAGE`, default `en`), recorded in the database on first start, and Dandori refuses to start if the setting later disagrees; switching language means a new database. The user shops at an Italian supermarket and needs food names that match the shelf, while the project is also shown as an English portfolio piece, so one installation runs in Italian and another in English. A single language per database is needed because an Ingredient is identified by its name: mixing languages would split one food into two Ingredients ("egg" and "uovo") across Recipes, Stock and the Grocery List.

## Considered Options

- **A language preference in the Profile, switchable**: needs translation on display or a migration on every switch, for a single user who never switches.
- **Bilingual storage** (every food term stored in both languages): doubles every name and still needs a rule for which one identifies the Ingredient.
- **Chosen once in the Profile at onboarding, switching empties the database**: the same guarantees as an installation setting, plus a form field, onboarding logic and a destructive flow nobody uses.

## Consequences

- The UI chrome, code, glossary and agent system prompts stay English; each prompt tells the model to write food terms in the Content Language and to translate pasted Recipes into it.
- Catalog Products are always in the supermarket's language; an English installation reaches them only through Product Mappings.
