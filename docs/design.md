# Design system

Dandori's look is **Pixel ledger**, the README banner's dusk brought to a
quiet ledger: sumi ink on near-white paper, with one vermilion seal for what
needs attention, framed by the banner's navy, its dithered sunset sky and its
pixel art. Space does the work that boxes and colour would do elsewhere; the
pixels stay at the edges (the shell, titles, controls), and what is read stays
flat paper.

Every page is built from the templ components in `internal/web`
(`components.templ`, `layout.templ`) and styled by one plain stylesheet,
`internal/web/static/app.css`, organised as tokens → base → components. There
is no CSS build step (ADR-0002).

## Tokens

Every colour, space and radius is a custom property on `:root` at the top of
`app.css`. That block is the only place a colour value may be written;
`internal/web/colours_test.go` fails CI on a hex or `rgb()` colour anywhere
else in `app.css` or in any `.templ` file. Tokens are named for their role, so
dark mode, out of scope for now, can later be one `prefers-color-scheme` block
that reassigns them (the test allows literals in any `:root` block).

| Token | Value | Use |
| --- | --- | --- |
| `--color-bg` | `#fdfcfa` paper | the page |
| `--color-surface` | `#ffffff` | raised things: the add and side sheets, trace details, an open `EditRow` |
| `--color-text` | `#1a1a18` sumi ink | text |
| `--color-text-soft` | `#5c5a54` soft ink | labels, hints, metadata |
| `--color-rule` | `#ecdfd2` warm peach-grey | hairlines between rows |
| `--color-rule-strong` | `#2d3352` navy | section rules, input underlines, borders of controls |
| `--color-fill` | navy | primary buttons, the chosen segment, a floating Notice |
| `--color-fill-dither` | `#4b5582` slate | dithered over the fill: the chosen segment, a button's hover |
| `--color-on-fill` | paper | text on a fill or on the accent |
| `--color-accent` | `#b8321b` vermilion seal | attention only (see below) |
| `--color-accent-wash` | `#fbe5dc` salmon wash | behind alerts |
| `--color-tag` | `#f7e4c6` cream | behind a `Tag` |
| `--color-veil` | navy | dithered over the page behind an open sheet |
| `--color-shell` | navy | the bottom tabs and the rail |
| `--color-on-shell` / `--color-on-shell-soft` | paper / `#b9bdd3` | the current tab / the other tabs |
| `--color-shell-marker` | `#eba083` salmon | the bar on the current tab |
| `--color-sky-*` | paper, cream `#f7e4c6`, peach `#f2c29c`, salmon `#eba083`, deep `#252840`, slate, snow `#f3eee4`, navy, sun (vermilion) | the sky and the pixel art only |

**The accent is reserved** for what needs attention: errors, things to use
soon, a page's primary add action (the `+`), section numbers and the stroke
under them, the seal on an urgent row, and destructive actions. The one
exception is the sun in the sky, as in the banner, which the seal echoes. Never use it
for decoration or emphasis. It is not the current tab's marker: vermilion on
navy is about 2:1, below the 3:1 a non-text indicator needs, so the marker is
salmon.

**Navy is structure**, not attention: it fills primary buttons, the chosen
segment and a floating Notice, draws section rules and input underlines, and is
the shell. The
banner's other colours (slate, cream, peach, salmon, snow) are only in the
sky and the pixel art, with three exceptions named above: slate in a
control's dither, cream behind a tag, salmon on the current tab and in the
alert's wash.

Type: three faces, self-hosted in `static/fonts` (SIL OFL,
`static/fonts/OFL.txt`). `--font-title` is DotGothic16, a pixel face, for
titles only: the page's `h1`, `Section` and `FieldGroup` headings (and any
`h2`), the sheet title and the wordmark. It is subset to Latin-1 and 段取り,
so a title in Japanese falls back to the system's face. `--font-body` is Zen
Kaku Gothic New (400, 700, latin subset) for everything else, `h3` in bold;
Japanese text falls back to the system's Gothic.

`--font-mono` is for code and trace details.

Space: `--space-1` … `--space-7` (0.25rem to 4rem). Shape: `--radius` is 0, so
every control, tag and the `+` is plainly square, with no stepped edges;
`--focus-ring` is the focus outline. Pixels: `--pixel` (3px) is one pixel of
the sky, `--pixel-fine` (2px) one of a control's dither. Size: `--tap`
(2.75rem, 44px) is the smallest touch target; `--measure` is the column's
width; `--rail-width` and `--tabs-height` size the shell.

`themeColour` in `internal/web/web.go` and the manifest's colours repeat
`--color-bg` because the browser can't read a custom property there;
`colours_test.go` checks they stay in step.

## Dithering

In place of gradients, colours meet in a **dither**: a stepped pattern of
square pixels of two colours, built in CSS from tokens, at four densities
(25, 50 and 75%, and solid). `--dither-on` is the colour laid over
`--dither-under`, and `--dither-pixel` the size of a pixel. A ramp steps from
one colour to the next through all four.

It may go in five places only:

- **the sky**: the masthead's dusk on a phone, cream to peach to salmon, then
  deep navy dithering down into the paper; the rail's, paper to salmon;
- **the veil** behind an open sheet: a navy checker over the page;
- **the chosen segment**: navy and slate;
- **a primary button's hover**: navy and slate;
- **a busy primary button**: navy and slate (see [Busy](#busy)).

Rows, forms and reading text stay flat paper. Smooth gradients are out (the
section rule is two flat strokes, not a blend).

## Pixel art and icons

The sun, Fuji and its tree line are bitmaps in `pixels.go`, rendered on the
server as inline SVG, one path per colour, each filled by a `px-*` class from a
`--color-sky-*` token. There is no runtime JS for presentation but two small
scripts in `Layout`: the busy button's (see [Busy](#busy)) and the floating
Notices' (see [Notices](#notices)). The sun and Fuji appear in the sky; the sun
alone is the **seal** (`Seal`) on an urgent row.

Icons are 10×10 bitmaps in `icons.go`, drawn 20px square, in the text colour
and `aria-hidden`. One that must be smaller is drawn 10px square: any other
size puts a bitmap pixel on a fraction of a screen pixel and blurs its edges.

## Layout

- **One calm column** (`--measure`, 42rem), with generous space above each
  section. No cards, no shadows, no boxes around content.
- **The shell** (`Layout`) is navy. On a phone, bottom tabs, each a pixel
  icon and a label in paper, with a salmon bar over the current one, and above
  the page the masthead: the dithered sky with the horizontal 段取り DANDORI
  wordmark linking Home, the pixel sun and Fuji, dithering down into the
  paper. From 900px, a left rail whose top is the same sky with the vertical
  wordmark, then the tabs, and Runs at the rail's foot. Before the Language is
  chosen there are no tabs: a phone shows no bar, desktop the rail's sky alone.
  Tabs are Plan, Recipes, Kitchen, Catalog and Profile; Runs is secondary. A page passes its
  `Tab` to `Layout`, which marks it `aria-current="page"`.
- **Every page opens with a `PageHeader`**: a letter-spaced eyebrow with the
  page's Japanese name and today's date (`在庫 · Sunday, October 25`) and a large
  pixel title, which is the page's only `h1`.
- **Content is grouped into numbered `Section`s** (`01 Stock Items`): a pixel
  heading over the banner's underline, a short vermilion stroke under the
  number, then navy. In a form, use `FieldGroup`, its fieldset twin.
- **Lists are `Rows`**: lines separated by hairlines, the name first,
  quantities in tabular numbers.

## Busy

A request that takes time shows it, the same way on every page, once it has
taken **300 ms**, so a quick click doesn't flicker; both go when it ends,
answered or failed. Neither needs anything from a page.

- **The busy bar**: a 5px bar across the whole top of the screen, over the
  rail on desktop, where navy and slate blocks march steadily to the right
  while any htmx request is in flight, boosted navigation included. It is
  `Layout`'s, shown by CSS while an element holds htmx's `htmx-request`.
- **The pressed button**: on top of being disabled (`hx-disable` on `<body>`),
  the button that sent the request, a form's submitter, is marked
  `aria-busy="true"`: a primary `Button`'s hover dither steps along, a
  `TextButton` or `DestructiveButton` turns soft ink with a dotted underline.
  CSS can't tell which button was pressed, so `Layout`'s `busyButton` script
  marks it: the one script for presentation.

Don't add an `hx-indicator` of a page's own for a slow request. Work that calls
the model is a Run, and shows its Trace instead.

## Notices

A **floating Notice** says something happened as asked ("Moved 300 g of rice
to the freezer."), the same way on every page. It never says something went
wrong: a refusal stays on its field, and a failure is an `Alert`.

- **Where**: at the top of the screen on every width, above everything, an
  open sheet included: sheets rise from the bottom on a phone, where the
  keyboard would cover it. On desktop it is centred over the column, right of
  the rail. Several stack, newest first.
- **What**: a navy block, the pixel check, paper text and a `Close` with the
  44px target; a paper line round its inside edge is its time, draining
  clockwise in steps. It drops in, and after about 5 seconds rises out and is
  gone. Hovering it, or focus in it, stops its time. `role="status"`, so it is
  announced.
- **How a page gives one**: a plain post answers with
  `web.RedirectWithNotice(w, r, path, text)`, and the page it lands on shows it
  once (a cookie the `web.Notices` middleware reads and forgets once a page
  renders, not on a redirect), without JS too; reloading that page doesn't
  show it again. An htmx answer wraps its fragment in
  `web.FragmentWithNotice(fragment, text)` (an `<hx-partial>` into `#notices`),
  or passes no fragment to only confirm. A page that confirms with no write, such
  as the Ingredient just picked, renders with `web.WithNotice(ctx, text)`.
- **Its script**: `Layout`'s `showNotices` raises the Notices into the top
  layer as a popover, after the sheets, as a page loads and after each swap.
  An open sheet makes the rest of the page inert, so they go inside the top
  sheet, still drawn over it, and leave it as it starts to close (on
  `beforetoggle`, while they still have their animations), keeping their time.
  They move before a swap adds a Notice, so it lands in a live region already
  in place, and is announced. It
  takes a Notice away when its time (the `notice-time` animation in `app.css`)
  ends, or on `Close`. With no JS the Notice still shows, stops while hovered
  and goes when its time runs out, all in CSS; only `Close` needs the script.
- **Closing a sheet with it**: an answer to a form in a sheet that is done, as a
  move is, triggers `web.CloseSheet` with `HX-Trigger`, and that sheet closes.

## Components

All in package `web`. Feature pages compose these; they don't style base
elements themselves.

| Component | Use it for |
| --- | --- |
| `Layout(title, tab)` | Every full page. Renders the head and the shell, whose tabs a page before signing in and choosing a Language (`SignedOut`) leaves out. |
| `PageHeader(ja, title, today)` | The top of every page; `today` is the app's clock's time, which the eyebrow shows as the date. Children, if any, are a `<p class="lede">` under the title. |
| `Section(num, title)` | A numbered group of a page's content. `num` 0 puts a small vermilion square in the number's place, for groups that aren't a page's own sequence, such as a side sheet's. |
| `FieldGroup(num, legend)` | A numbered group of a form's fields; `num` 0 marks it as `Section`'s does. |
| `FieldGroupAction(id, num, legend, action)` | A `FieldGroup` with a secondary action beside its legend, for one that changes what the legend names (the Count form's `Choose another`); on a long legend it drops to the next line, at the far end. `id` is the legend text's, which alone names the group. |
| `Hint()` | A quiet paragraph explaining what follows. |
| `Empty()` | What a list says when it has nothing in it. |
| `Disclosure(summary)` | A group of a page's content a click away (the day's later Slots on Today): a quiet summary line with a chevron, closed until opened. |
| `Fold(summary, holds)` | A line that opens in place to show what it holds (a Trace Event's data): a chevron at its start points right while closed and down while open, and it starts closed. Without `holds` it is the line alone, aligned with its neighbours. |
| `Rows()` | A list of `Row`s or `EditRow`s. |
| `Row()` | A read-only line. Lay it out with spans of class `row-title` (the name, or a link), `row-figure` (a quantity), `row-meta` (a second line of details) and a `row-action` form. |
| `Seal()` | The pixel sun before an urgent `Row`'s `row-title`. Decorative: the row's text (an accent `Tag`) says why it is urgent. |
| `EditRow(open, summary)` | A line that opens in place (`<details>`) to edit, raised on the surface while open: `summary` is the line, children the edit form. Pass `open` when the form was rejected, so its errors show. |
| `EditRowIn(group, open, summary)` | An `EditRow` one of a group, such as the Slots of a day: opening it closes the group's open one. |
| `Field(label, hint, err)` | Any labelled control: the control is the children; `hint` follows the label in brackets (`optional`, a unit); `err` shows under it. |
| `SearchField(label)` | The search box over a list, the input the child: a square box with the magnifying glass inside, so it doesn't read as one of the rows. `label` is for a screen reader; the input's placeholder says it on screen. |
| `ListSearch(id, action, listURL, listID, label)` | A `SearchField` in a form that swaps the list as the User types and opens `action` searched without htmx, the input the child. |
| `TimeField(label, name, value, err)` | A `Field` for a time of day (a Slot time, a Delivery window), typed in either 24- or 12-hour form and shown in the Language's (`web.TimeOfDay`); never a native `type="time"`, which follows the browser's locale. |
| `DateField(label, name, value, err)` | A `Field` for a day, shown in the Language's order (`domain.FormatDate`); never a native `type="date"`. |
| `CellField(label, err)` | A `Field` in a grid whose column heads name it; the label shows only on a phone. |
| `Check(label, hint)` | A checkbox, the child, before its label (marking an Ingredient a Staple); the whole line is its target. |
| `FieldError(err)` | An error outside a `Field`, e.g. for a whole list. |
| `FieldRow()` | A few short `Field`s side by side, wrapping on a phone. Their controls line up even when one label wraps onto two lines. |
| `Actions()` | The row of buttons ending a form; the primary one first. |
| `ActionsApart()` | An `Actions` with one more action that isn't part of the form, at the far end of the row, always in the same place (the Count form's `Movements`). |
| `Button(attrs)` | The primary action of a form, in navy, dithering with slate on hover. One per form. |
| `ButtonLink(href)` | A link that is the primary action (e.g. Edit). |
| `TextButton(attrs)` | A secondary action, underlined text. |
| `TextLink(attrs)` | A secondary action that writes nothing, looking like a `TextButton`. Unlike one in a form, it isn't what Enter presses, and the form's required fields don't hold it up. |
| `DestructiveButton(attrs)` | Remove, delete, discard, cancel: vermilion text. |
| `Segmented(legend, name, choices, selected, err)` | Choosing one of two to four options at a glance (moving a Portion). For more, or in a long form, use a `<select>`. |
| `SegmentedLinks(label)` | A `Segmented` whose options each open something instead of filling in a form, so what is shown is reachable from the others (a Stock Item's Movements in each Storage Location). Children are `SegmentLink`s; `label` is for a screen reader. |
| `SegmentLink(current, attrs)` | One option of `SegmentedLinks`, marked `aria-current` and shown as the chosen segment when `current`. `attrs` give its `href`, and in a sheet the swap of its body. |
| `Alert()` | Something went wrong: a rejected form, a failure. On the salmon wash, with a vermilion edge. |
| Floating Notice | Something happened as asked ("Profile saved."), floating at the top of the screen: `web.RedirectWithNotice` for a plain post, `web.FragmentWithNotice` for an htmx answer, `web.WithNotice` for a page. `Layout` shows it; see [Notices](#notices). There is no in-flow notice: one in the page or sheet it confirms pushed the form down, and the list changing behind a sheet may be off-screen. |
| `Tag(accent, title)` | A short label on a row (`New`, a Run's status), a flat cream fill. `accent` when it needs attention: vermilion with paper text. |
| `AddButton(choices)` | A page's primary add action, one per page: a square vermilion `+` opening the sheet of the first `AddChoice`. One choice is a plain `+`. With more, the `+` shows the icon of what it adds, and a chevron joined to it opens a menu (a native popover of radios, so it needs no JS) that chooses another: choosing changes what the `+` adds and opens nothing, so the `+` always says what it will do. The first choice is the most frequent, and a new page starts on it again. An `AddChoice` whose sheet doesn't hold its body gives `Src` and `Href`, as a `SideSheetLink` does. One with only `Href` is a link to a page, for a form too long for a sheet. |
| `AddSheet(id, title, open)` | The sheet of an add form: from the bottom on a phone, centred on desktop, over a navy dithered veil. Every add form is one, whichever choice of the `AddButton` opens it. Every sheet is modal, a `<dialog>` opened with `showModal()`: the page behind it is inert and holds still, Tab stays in it, and a tap on the veil or Esc closes it, the top one where one sheet is open over another. A sheet taller than the screen scrolls itself, and only itself. A tap outside discards a half-filled form, without asking. Pass `open` when the form was rejected: it reopens with focus on the first field marked `aria-invalid`. |
| `SideSheet(id, title)` | A sheet of content any page can open without leaving it (the Ingredients): from the right, the screen's full height and up to 60% of its width, on desktop; from the bottom on a phone, as `AddSheet`'s; over the same navy dithered veil, and modal as every sheet is (see `AddSheet`). Its body is empty until opened. A page renders it once, then opens it with `SideSheetLink`s. |
| `SheetBody(id)` | The body of a sheet whose content swaps in place: every `SideSheet`'s, and an `AddSheet`'s that keeps working through several saves, as the Kitchen's Count does, or that loads as it opens, as Add Portions does. A form in it swaps it (`hx-target` on `SideSheetBody(id)`); a rejected one comes back with focus on its first field marked `aria-invalid`, or else the one marked `autofocus`, the sheet staying open. A swap into part of the body, such as a search's results, leaves the focus where it is. |
| `SectionBody(id)` | A section of a page whose form swaps it in place, the form the children: each of the Profile's sections, saved on its own. The form targets `#id` with `innerHTML`; a rejected one comes back with focus on its first field marked `aria-invalid`. |
| `SideSheetLink(id, href, src)` | Opens the sheet `id`, loading its `SheetBody` from `src`, a fragment, and focuses its field marked `autofocus`, or else its first. Without htmx it is a plain link to `href`, a page of the same content. Placed inside another sheet, it opens over that sheet, which stays open beneath it. |
| `SideSheetLinkWith(id, href, src, attrs)` | A `SideSheetLink` with more attributes, such as an `hx-include` adding to `src` what the page around it says at that moment. |
| `LanguageSwitch(u)` | The pages before a User has chosen their Language (sign-in, the Language choice): a link to each Language, named in itself, staying at `u`. The choice is remembered in this browser. |
| `Markdown(text)` | Text an agent wrote in Markdown, such as a Run's notes. Raw HTML in it is left out. |
| `Icon(name)` | A 10×10 pixel icon, inline SVG in the text colour. Draw new ones in `icons.go`; an unknown name fails the render. |

Two layout classes go on elements a component can't wrap, such as a
`<form>`: `stack` spaces its children evenly in a column (a form's fields and
alert), and `section` gives an unnumbered group a `Section`'s spacing.

Base elements (`input`, `select`, `textarea`, `table`, `dl`, `h1`–`h3`, lists)
are styled once in `app.css`: underline-only inputs, square bordered
textareas, tables ruled like rows, `dl` as a two-column list of facts.

Three feature-shaped components live in `app.css` because nothing else needs
them yet: the Recipe form (`ingredient-grid`, `ingredient-row`, `ingredient-details`, `draft-steps`), the
Run `trace` panel, and the Exceptions page's `slot-grid`, a Cycle's Slots as
rows on a phone and a day × Slot grid on desktop, where an open Slot spans
its day over the day's other Slots; `slot-grid.js` animates a Slot opening
and closing.

## Accessibility baseline

- Text and controls meet WCAG AA contrast. On paper: ink 17:1, soft ink
  6.7:1, accent 5.8:1, navy 12:1. On navy: paper 12:1, the other tabs'
  `#b9bdd3` 6.6:1, and paper on slate (the lighter of a control's dither) 7:1.
  Paper on accent 5.8:1, accent on its salmon wash 4.9:1, ink on a cream tag
  14:1.
- Non-text indicators reach 3:1: the current tab's salmon bar is 5.8:1 on
  navy, a section rule and an input underline 12:1 on paper.
- Touch targets are at least `--tap` (44px).
- Focus is always visible: a 2px vermilion ring on every control, inputs
  included, so a focused field stands out from an invalid one. On the navy
  tabs and a floating Notice the ring is salmon (5.8:1), as vermilion there is
  about 2:1.
- `prefers-reduced-motion` turns off transitions and animations: the busy
  bar's blocks and a busy button's dither stand still.
  The bar still waits its 300 ms. A floating Notice appears and goes with no
  transition, its line still and whole, and still goes when its time runs out.
- Meaning is never carried by colour alone: an error says what is wrong and
  is marked `!`, an invalid field has a thicker underline and
  `aria-invalid`, an urgent row has the pixel seal *and* text, the current tab
  has a bar, paper text and `aria-current`.
- Icons, the seal and the pixel art are `aria-hidden`; what they mean is said
  in text or an `aria-label`.

## Do and don't

- **Do** build a page from the components above; add a component to
  `internal/web` when a piece will recur, and catalogue it here.
- **Do** use tokens for every colour, space and radius.
- **Do** let space separate things: hairlines and section rules, not boxes.
- **Do** keep dithering to the sky, the veil, the chosen segment, a
  button's hover and a busy button, and the banner's colours other than
  navy to the sky and the pixel art.
- **Do** give every form one primary `Button`; everything else is a
  `TextButton` or `DestructiveButton`.
- **Don't** write a colour value outside the token block, or in a template.
- **Don't** restyle base elements for one feature (`.profile input`,
  `.stock-form select`): change the base style or add a component.
- **Don't** use the accent for anything that doesn't need attention.
- **Don't** add rounded corners, stepped corners, shadows, smooth gradients or
  a second accent.
- **Don't** add an icon font, a stroke icon or runtime JS for presentation
  (the busy button's and the floating Notices' scripts are the exceptions).
- **Don't** set body text, labels or rows in the pixel face.
