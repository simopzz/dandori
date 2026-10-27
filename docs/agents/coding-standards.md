# Coding standards

Rules code review checks against. Each says what it protects.

## UI

- **Pages are built from the shared templ components** in `internal/web`
  (`Layout`, `PageHeader`, `Section`, `Rows`, `Field`, `Button`, … catalogued
  in [`docs/design.md`](../design.md)). A recurring piece of UI becomes a
  component there, not a copy in each feature. *Protects: one look across
  pages, and new pages built in it.*
- **Colours, spaces and radii are tokens.** No raw colour value (hex,
  `rgb()`) in a `.templ` file or in `app.css` outside its `:root` token block;
  `internal/web/colours_test.go` enforces it in CI. Spaces and radii use
  `--space-*` and `--radius`. *Protects: a palette changed in one place, and
  dark mode as one block later.*
- **Pages speak the glossary.** What a template shows uses `GLOSSARY.md`'s
  terms, never a word it lists under _Avoid_;
  `internal/web/glossary_test.go` enforces it in CI. A word used in its
  ordinary sense, not as the avoided synonym, goes in that test's
  `ordinarySense` list with the reason. *Protects: the user reads the same
  words the model and the code use.*
- **No per-feature restyling of base elements.** No selectors like
  `.profile input` or `.stock-form select` in `app.css`: change the base style
  for everyone, or add a component. The exceptions are the feature components
  `docs/design.md` names (the Recipe Draft form's `ingredient-grid` and
  `draft-steps`, the Run `trace` panel); a new one is catalogued there first. *Protects: a form control looks and
  behaves the same on every page.*
- **The accessibility baseline in `docs/design.md` holds**: AA contrast,
  44px touch targets, visible focus, reduced motion, and meaning never carried
  by colour alone.
