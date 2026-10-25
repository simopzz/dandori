# Design system

Dandori's look is **"Ma on white paper"**: Japanese-minimal, a quiet ledger
in sumi ink on near-white paper, with one vermilion seal for what needs
attention. Space does the work that boxes and colour would do elsewhere.

Every page is built from the templ components in `internal/web`
(`components.templ`, `layout.templ`) and styled by one plain stylesheet,
`internal/web/static/app.css`, organised as tokens → base → components. There
is no CSS build step (ADR-0002).

## Tokens

Every colour, space and radius is a custom property on `:root` at the top of
`app.css`. That block is the only place a colour value may be written;
`internal/web/colours_test.go` fails CI on a hex or `rgb()` colour anywhere
else in `app.css` or in any `.templ` file. Tokens are named for their role, so
dark mode can later be one `prefers-color-scheme` block that reassigns them.

| Token | Value | Use |
| --- | --- | --- |
| `--color-bg` | `#fdfcfa` paper | the page |
| `--color-surface` | `#ffffff` | raised things: the add sheet, trace details |
| `--color-text` | `#1a1a18` sumi ink | text |
| `--color-text-soft` | `#5c5a54` | labels, hints, metadata |
| `--color-rule` | `#e4e0d8` | hairlines between rows |
| `--color-rule-strong` | ink | under section headings, input underlines, borders of controls |
| `--color-fill` / `--color-fill-hover` | ink / black | primary buttons, the chosen segment |
| `--color-on-fill` | paper | text on a fill or on the accent |
| `--color-accent` | `#b8321b` vermilion seal | attention only (see below) |
| `--color-accent-wash` | `#faebe6` | behind alerts |
| `--color-backdrop` | ink at 35% | behind an open sheet |

**The accent is reserved** for what needs attention: errors, things to use
soon, the current tab, a page's primary add action (the `+`), section
numbers, and destructive actions. Never use it for decoration or emphasis.

Type: `--font-display` is Shippori Mincho (500, 700) for headings,
`--font-body` is Zen Kaku Gothic New (400, 700) for everything else. Both are
latin subsets, self-hosted in `static/fonts` (SIL OFL, `static/fonts/OFL.txt`);
Japanese text falls back to the system's Mincho or Gothic.

Space: `--space-1` … `--space-7` (0.25rem to 4rem). Shape: `--radius` is 0, so
every control is square. Size: `--tap` (2.75rem, 44px) is the smallest touch
target.

`themeColour` in `internal/web/web.go` and the manifest's colours repeat
`--color-bg` because the browser can't read a custom property there; keep
them in step.

## Layout

- **One calm column** (`--measure`, 42rem), with generous space above each
  section. No cards, no shadows, no boxes around content.
- **The shell** (`Layout`): on a phone, bottom tabs, each an icon and a label,
  with a vermilion bar over the current one. From 900px, a left rail with the
  vertical 段取り wordmark over DANDORI, the tabs, and Runs at the rail's foot.
  Tabs are Recipes, Stock and Profile; Runs is secondary. A page passes its
  `Tab` to `Layout`, which marks it `aria-current="page"`.
- **Every page opens with a `PageHeader`**: a letter-spaced eyebrow with the
  page's Japanese name and today's date (`在庫 · Sunday 25 October`) and a large
  serif title, which is the page's only `h1`.
- **Content is grouped into numbered `Section`s** (`01 Stock Items`): a serif
  heading over a strong rule. In a form, use `FieldGroup`, its fieldset twin.
- **Lists are `Rows`**: lines separated by hairlines, the name first,
  quantities in tabular numbers.

## Components

All in package `web`. Feature pages compose these; they don't style base
elements themselves.

| Component | Use it for |
| --- | --- |
| `Layout(title, tab)` | Every full page. Renders the head and the shell. |
| `PageHeader(ja, title)` | The top of every page. Children, if any, are a `<p class="lede">` under the title. |
| `Section(num, title)` | A numbered group of a page's content. |
| `FieldGroup(num, legend)` | A numbered group of a form's fields. |
| `Hint()` | A quiet paragraph explaining what follows. |
| `Empty()` | What a list says when it has nothing in it. |
| `Rows()` | A list of `Row`s or `EditRow`s. |
| `Row()` | A read-only line. Lay it out with spans of class `row-title` (the name, or a link), `row-figure` (a quantity), `row-meta` (a second line of details) and a `row-action` form. |
| `EditRow(open, summary)` | A line that opens in place (`<details>`) to edit: `summary` is the line, children the edit form. Pass `open` when the form was rejected, so its errors show. |
| `Field(label, hint, err)` | Any labelled control: the control is the children; `hint` follows the label in brackets (`optional`, a unit); `err` shows under it. |
| `CellField(label, err)` | A `Field` in a grid whose column heads name it; the label shows only on a phone. |
| `FieldError(err)` | An error outside a `Field`, e.g. for a whole list. |
| `FieldRow()` | A few short `Field`s side by side, wrapping on a phone. |
| `Actions()` | The row of buttons ending a form; the primary one first. |
| `Button(attrs)` | The primary action of a form, in ink. One per form. |
| `ButtonLink(href)` | A link that is the primary action (e.g. Edit). |
| `TextButton(attrs)` | A secondary action, underlined text. |
| `DestructiveButton(attrs)` | Remove, delete, discard, cancel: vermilion text. |
| `Segmented(legend, name, choices, selected, err)` | Choosing one of two to four options at a glance (moving a Portion). For more, or in a long form, use a `<select>`. |
| `Alert()` | Something went wrong: a rejected form, a failure. On the accent wash. |
| `Notice()` | Something happened as asked ("Profile saved."). |
| `Tag(accent, title)` | A short label on a row (`New`, a Run's status). `accent` when it needs attention. |
| `AddSheet(id, title, open)` | A page's primary add action: a square vermilion `+` opening the add form as a popover sheet, from the bottom on a phone, centred on desktop. Pass `open` when the form was rejected. One per page. |
| `Icon(name)` | A Lucide icon, inline SVG in the text colour. Add icons to `icons.go`. |

Base elements (`input`, `select`, `textarea`, `table`, `dl`, `h1`–`h3`, lists)
are styled once in `app.css`: underline-only inputs, square bordered
textareas, tables ruled like rows, `dl` as a two-column list of facts.

Two feature-shaped components live in `app.css` because nothing else needs
them yet: the Recipe Draft form's `ingredient-grid` and the Run `trace`
panel.

## Accessibility baseline

- Text and controls meet WCAG AA contrast on `--color-bg` (ink 17:1, soft ink
  6.7:1, accent 5.8:1, paper on accent 5.8:1, accent on its wash 5.2:1).
- Touch targets are at least `--tap` (44px).
- Focus is always visible: a 2px vermilion ring, or a 2px vermilion underline
  on inputs.
- `prefers-reduced-motion` turns off transitions and animations.
- Meaning is never carried by colour alone: an error says what is wrong and
  is marked `!`, an invalid field has a thicker underline and
  `aria-invalid`, an urgent row needs a seal mark *and* text, the current tab
  has a bar and `aria-current`.
- Icons are `aria-hidden`; what they mean is said in text or an `aria-label`.

## Do and don't

- **Do** build a page from the components above; add a component to
  `internal/web` when a piece will recur, and catalogue it here.
- **Do** use tokens for every colour, space and radius.
- **Do** let space separate things: hairlines and section rules, not boxes.
- **Do** give every form one primary `Button`; everything else is a
  `TextButton` or `DestructiveButton`.
- **Don't** write a colour value outside the token block, or in a template.
- **Don't** restyle base elements for one feature (`.profile input`,
  `.stock-form select`): change the base style or add a component.
- **Don't** use the accent for anything that doesn't need attention.
- **Don't** add rounded corners, shadows, gradients or a second accent.
- **Don't** add an icon font or runtime JS for presentation.
