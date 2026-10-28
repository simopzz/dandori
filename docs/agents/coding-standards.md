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

## htmx

- **htmx 4, not htmx 2.** The vendored `htmx.min.js` is 4.0.0: inheritance
  is explicit (`hx-target:inherited`), `hx-disable` disables elements during
  a request (htmx 2's `hx-disabled-elt`), events are colon-named
  (`htmx:after:swap`), and every status but 204/304 swaps. The
  `htmx-guidance` skill is the reference; `internal/web/htmx_test.go`
  rejects htmx 2's names in CI. *Protects: markup that silently does
  nothing under the version we ship.*
- **Pages are boosted; a targeted fragment only keeps your place.** Links
  and forms ride the `hx-boost` on `<body>`, and handlers answer the same
  way for htmx and a plain browser: redirect after a write, re-render the
  page with 422 on a rejection. A form swaps a fragment of its own
  (`httpx.IsHTMX`) only when the boosted jump to the top would lose the
  user's place, such as a row deep in a list. See
  [ADR 0009](../adr/0009-boosted-pages-with-fragments-only-to-keep-your-place.md).
  *Protects: one way to answer a request, and fragment code only where it
  pays.*
- **Every htmx request has a no-JS path.** A form keeps `method` and
  `action`, which `internal/web/htmx_test.go` checks. *Protects: the app
  works before htmx loads and when it fails.*
- **Validation errors come back with 422 and what was typed**, as the page
  or as the form's fragment. *Protects: input is never lost, and tests can
  tell failure from success.*
- **A write can't be submitted twice.** `hx-disable` on `<body>` disables a
  form's buttons while its request is in flight; a form that opts out of
  the boost keeps the same. A request slow in itself shows an
  `hx-indicator`; work that calls the model runs as a Run and shows its
  Trace instead. *Protects: no duplicate writes, and the user knows
  something is happening.*
- **Use a shipped extension before writing JS.** Streaming, preloading and
  similar behaviour come from htmx's own extensions (`hx-sse.js` for SSE),
  vendored next to `htmx.min.js`; a JS island is for what htmx can't do
  (Cooking Mode's Wake Lock and timers). *Protects: one way to talk to the
  server, and less hand-rolled code to maintain.*
