# Design system

Dandori's look is **Pixel ledger**, the README banner's dusk brought to a
quiet ledger: sumi ink on near-white paper, with one vermilion seal for what
is wrong, urgent or irreversible, framed by the banner's navy, its dithered sunset sky and its
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
| `--color-rule-strong` | `#2d3352` navy | section numbers, strokes and rules, field boxes, borders of controls, the focus ring |
| `--color-fill` | navy | primary buttons, the chosen segment, a floating Notice |
| `--color-fill-dither` | `#4b5582` slate | dithered over the fill: a button's hover, a busy button |
| `--color-on-fill` | paper | text on a fill or on the accent |
| `--color-accent` | `#b8321b` vermilion seal | wrong, urgent or irreversible only (see below) |
| `--color-accent-wash` | `#fbe5dc` salmon wash | behind alerts |
| `--color-tag` | `#f7e4c6` cream | behind a `Tag`, and a value worth weighing |
| `--color-tag-done` | `#d3e5cc` sage | behind a `ToneTag("done")`, a succeeded Run |
| `--color-veil` | navy | dithered over the page behind an open sheet |
| `--color-shell` | navy | the bottom tabs and the rail |
| `--color-on-shell` / `--color-on-shell-soft` | paper / `#b9bdd3` | the current tab / the other tabs |
| `--color-shell-marker` | `#eba083` salmon | the bar on the current tab |
| `--color-sky-*` | paper, cream `#f7e4c6`, peach `#f2c29c`, salmon `#eba083`, deep `#252840`, slate, snow `#f3eee4`, navy, sun (vermilion) | the sky and the pixel art only |

**The accent means wrong, urgent or irreversible**, and nothing else: errors
and a refused field's box, an urgent row's seal and its accent `Tag`,
and destructive actions. It also stays on the `+`, which is the seal, and on
the sun in the sky, as in the banner, which the seal echoes. Never use it for
decoration, emphasis, structure or a soft miss: a page where everything is vermilion has
nothing left for an error to stand out against. It is not the current tab's
marker: vermilion on navy is about 2:1, below the 3:1 a non-text indicator
needs, so the marker is salmon.

**Worth weighing is cream.** What the User should weigh but needn't fix, a soft
miss or a change, is marked on a cream `Tag` with a shape that says which way:
the `under` icon (▼) under a Target, `over` (▲) over it, `changed` (•) changed,
each followed by its word for a screen reader (`sr-only`). Vermilion stays for a
broken hard rule and a failure.

**Navy is structure**, not attention: it fills primary buttons, the chosen
segment and a floating Notice, draws section numbers, the stroke under them,
section rules, field boxes and the focus ring, and is the shell. The
banner's other colours (slate, cream, peach, salmon, snow) are only in the
sky and the pixel art, with three exceptions named above: slate in a
control's dither, cream behind a tag and a value worth weighing, salmon on the
current tab and in the alert's wash.

Type: three faces, self-hosted in `static/fonts` (SIL OFL,
`static/fonts/OFL.txt`). `--font-title` is DotGothic16, a pixel face, for
titles only: the page's `h1`, `Section` and `FieldGroup` headings (and any
`h2`), the sheet title and the wordmark. It is subset to Latin-1 and 段取り,
so a title in Japanese falls back to the system's face. `--font-body` is Zen
Kaku Gothic New (400, 700, latin subset) for everything else, `h3` in bold;
Japanese text falls back to the system's Gothic. `Layout` preloads all three.
The body faces are `font-display: optional`: text rewrapping as a face swapped
in moved the page (a layout shift), so a body face that isn't there for the
first paint waits for the next page. The title face swaps in.

A title's data, its times, quantities and User text (Today's `1:00 pm`, a
Recipe's or an Ingredient's name), is set in the body face, where the pixel
face makes l, I and 1 hard to tell apart: wrap it in `web.Data` where the
title is made (`web.Tf(ctx, msgNextAt, kind, web.Data(at))`), and
`PageHeader`, `Section` and `FieldGroup` set it so.

`--font-mono` is for code and trace details.

Nothing a User reads to act is under 0.875rem: field labels, errors, tab
names, tags, a row's details, table and grid heads. Only the eyebrow and the
wordmark are smaller. Segment labels are 1rem. Labels stay soft ink, 6.7:1 at
0.875rem. Letter-spacing is for the eyebrow, the wordmark and the titles in
the pixel face (0.04em); labels, buttons, tabs, tags and running text keep
the body's, as tracking breaks up a word's shape. `type_test.go` checks both.

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

It may go in four places only:

- **the sky**: the masthead's dusk on a phone, cream to peach to salmon, then
  deep navy dithering down into the paper; the rail's, paper to salmon;
- **the veil** behind an open sheet: a navy checker over the page;
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
The `FieldInfo` mark is a small stroked vector icon, so its circle and letter stay
smooth at every size.

**An icon before words sits on the middle of their lowercase letters**, not on
the middle of their line nor of their capitals: most of what follows an icon is
lowercase, and an icon centred higher reads as sitting high. One rule in
`app.css` places it, for a `Row` led by its icon and a `TextLink` led by one:
the line aligns on baselines and the icon moves down half its height less half
an `ex`. An icon placed before words anywhere else joins that rule.

**A kind of line has a fixed icon**, its meaning in words beside it or
`sr-only`, the same on every page: `under` (▼), `over` (▲) and `changed` (•)
for what is worth weighing, `repeat` for a Portion eaten back to back,
`package` (the Kitchen's crate) for a Portion left unused, `snowflake` for one
made for after the Cycle and for the freezer splits at put-away, `scale` for the
days more than 25% off a Target, `book-open` for a new Recipe, and `list` for
the Grocery List. A reminder on
Today leads with the icon of the page it links to, its tab's: `calendar` (Plan)
for opening a Cycle and for how planning one stands, `list` for ordering the
next Delivery from its Grocery List, `user` (Profile) for the Targets planning
needs; a failed one, urgent, has the seal instead. A new kind of
line gets its own icon here before it lands on a page.

## Layout

- **One calm column** (`--measure`, 42rem), with generous space above each
  section. No cards, no shadows, no boxes around content.
- **The shell** (`Layout`) is navy. On a phone, bottom tabs, each a pixel
  icon and a label in paper, with a salmon bar over the current one, and above
  the page the masthead: on Today, the full sky, the dithered dusk with the
  horizontal 段取り DANDORI wordmark linking Home, the pixel sun and Fuji,
  dithering down into the paper; on every other page a slim strip of it,
  about 48px, with the wordmark alone, so the page's content starts high. From 900px, a left rail whose top is the same sky with the vertical
  wordmark, then the tabs, and Runs at the rail's foot. Before the Language is
  chosen there are no tabs: a phone shows no bar, desktop the rail's sky alone.
  Tabs are Today, Plan, Recipes, Kitchen and Profile; a later step adds Catalog.
  Runs is secondary: at the rail's foot on desktop, and on a phone not a tab but
  a link on the Profile. A page passes its `Tab` to `Layout`, which marks it
  `aria-current="page"`.
- **Every page opens with a `PageHeader`**: a letter-spaced eyebrow with the
  page's Japanese name and today's date (`在庫 · Sunday, October 25`) and a large
  pixel title, which is the page's only `h1`.
- **Content is grouped into numbered `Section`s** (`01 Stock Items`): a pixel
  heading over the banner's underline, the number and a short stroke under
  it in navy, a gap, then the navy rule. In a form, use `FieldGroup`, its
  fieldset twin. A page numbers all its own groups, in order; the square in
  the number's place is for a side sheet's.
- **The heading ladder** is three steps and no more: the page's `h1` is its
  `PageHeader`, an `h2` a `Section`, an `h3` a `Group`. Below a `Group`,
  content is `Rows`, captioned by a `RowsHead` where a Group holds more than
  one list.
- **Lists are `Rows`**: lines separated by hairlines, the name first,
  quantities in tabular numbers.
- **Tables** carry each column's unit and Target in its head ("Protein, g" over
  "≥ 130"), so the cells hold numbers alone. A bound is written ≥ or ≤, with
  "at least" or "at most" for a screen reader; a value known only as a floor
  is "29+", and an unknown one a dash, both through `Amount`. Rows that repeat
  collapse into a range ("Fri, Oct 30 – Thu, Nov 5: no Plan"). A date in a
  cell stays on one line, and on a phone the columns sit 8px apart, the last
  with no gap at the edge, so a table of four figures fits 360px. A table wider
  than the column scrolls in a box of its own, its first column sticky, never
  the page.
- **Dates**: the long form in headings (`web.Date`, "Friday, November 6"), the
  short form in rows and tables (`web.ShortDate`, "Fri, Nov 6"), and the year
  only when it isn't this one (`web.ShortDateInYear`).
- **How a row says it opens**, the same everywhere: a name that goes
  somewhere is underlined, as a link, whether to a page or a sheet (a sheet
  has a page of its own without JS); what opens inside the row (`EditRow`,
  `FoldRow`, `Disclosure`, `Fold`, `Group`) has a chevron instead.

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

## Refusals

- **The server's refusal is the only error a User sees.** Every `<form>` has
  `novalidate` (a guardrail checks), so the browser never stops a post with its
  own bubble, unstyled and in the browser's language rather than the User's.
  Controls keep `required` for what it tells a screen reader. The post is
  refused with a 422, each refused field marked `aria-invalid` with why under
  it, in the User's Language.
- **A rejected form longer than a screen opens with an `ErrorSummary`**: an
  `Alert` listing each refusal, in the form's order, as a link to its field,
  focused as the page loads (the GOV.UK error summary). Only the full-page
  forms have one: the Recipe form, written, edited or checked from a Recipe
  Draft, and the Add Portions page. A form in a sheet, or a short one (a
  Count, a Move, an Ingredient), focuses its first refused field instead.

## Components

All in package `web`. Feature pages compose these; they don't style base
elements themselves.

| Component | Use it for |
| --- | --- |
| `Layout(title, tab)` | Every full page. Renders the head and the shell, whose tabs a page before signing in and choosing a Language (`SignedOut`) leaves out. |
| `PageHeader(ja, title, today)` | The top of every page; `today` is the app's clock's time, which the eyebrow shows as the date. Children, if any, are a `<p class="lede">` under the title. |
| `PageHeaderBeside(ja, title, beside, today)` | A `PageHeader` whose title has `beside` after it, in the body face and smaller, what the page is of ("Cycle · Thu, Nov 5 – Sat, Nov 14"); on a phone it wraps under the title. |
| `SideColumn(side)` | A page with a decision beside a long list (a Plan Proposal beside its days): the children are the main column, and `side` comes first on a phone; from 1200px `side` is a column of its own on the right, sticky, so its actions stay in view while the main column scrolls, and the page widens to hold both. |
| `Section(num, title)` | A numbered group of a page's content. `num` 0 puts a small navy square in the number's place, for groups that aren't a page's own sequence, such as a side sheet's. |
| `FieldGroup(num, legend, hint)` | A numbered group of a form's fields; `num` 0 marks it as `Section`'s does. `hint` follows the legend in brackets, as a `Field`'s does: `optional` for a group that may be left empty (a Recipe's Steps). |
| `FieldGroupAction(id, num, legend, action)` | A `FieldGroup` with a secondary action beside its legend, for one that changes what the legend names (the Count form's `Choose another`); on a long legend it drops to the next line, at the far end. `id` is the legend text's, which alone names the group. |
| `Hint()` | A quiet paragraph explaining what follows. |
| `Empty()` | What a list or a page says when it has nothing in it: it says what would be here, why it isn't, and links the one action that fills it. Today, before it has a Plan, names each thing planning needs instead, each a link named for its action. |
| `Disclosure(summary, open)` | A group of a page's content a click away (the day's later Slots on Today, the add form above the Ingredients side sheet's list): a quiet summary line with a chevron, closed until opened, or open when it holds what the User is at, such as a refused form. |
| `Fold(summary, holds)` | A line that opens in place to show what it holds (a Trace Event's data): a chevron at its start points right while closed and down while open, and it starts closed. Without `holds` it is the line alone, aligned with its neighbours. |
| `Group(title, meta, open)` | A part of a `Section` that folds: a `<details>` whose summary is an `h3`, `title`, with `meta`, a component or nil, under it (a soft line, or a day's `Figures` and what it cooks) and a chevron at the end, over a 1px navy rule, weaker than a Section's and stronger than a row's hairline (a day of a Plan). Closed unless it holds what the User is at, such as a refused form. It holds `Rows`, never a deeper heading. |
| `RowsHead(label)` | A small soft caption over a `Rows`, on a 1px navy line (a day's Slots, then its Prep Sessions). |
| `Rows()` | A list of `Row`s, `EditRow`s or `FoldRow`s. |
| `Row()` | A read-only line. Lay it out with spans of class `row-title` (the name, or a link), `row-figure` (a quantity), `row-meta` (a second line of details) and a `row-action` form. With an action, from 600px the title, the figure right after it and the action at the end share the first line, and the details are the line under them. A kind of line (a soft miss) leads with its `Icon`, and its `row-meta` lines up under the title. |
| `Figures()` | A line of figures in equal columns, the children each a cell, lined up down a `Section` under a `Figures` of their heads (class `figures figures-head` on the head line): a day's four values against the Targets, without a table, so a day's row can fold as a `Group`. A head names its value and unit over its Target ("Protein, g" over "≥ 130"); a cell holds `Amount`, its name `sr-only`. |
| `RowTitle(name, tagged, tag, changed)` | A `Row`'s `row-title`: the name, and when `tagged`, `tag` after it as a plain `Tag` (a `New` Ingredient, a "new Recipe"), then `Changed` when a Plan Proposal `changed` it. |
| `SlotRow(kind, at, portion, changed, size, figures, sources...)` | A Slot and the Portion filling it, as a `Row`: the Portion as its `RowTitle`, marked when `changed`; its `size` and `figures` (calories and protein through `Amount`) as the `row-figure`; the Slot's kind, its time `at` where shown (Today), and `sources`, words for where the Portion comes from ("cooked", "from Thu, in the fridge", "already in the kitchen", "new Recipe", "start at 12:35 pm"), as its `row-meta`. Leave out what isn't shown with `""` or a nil `figures`. `SlotLine` is the same line, the summary of a `FoldRow` that opens to the Slot's Prep Session. |
| `QuantityFigure(q, unit)` | A `Row`'s `row-figure` holding a quantity in its Ingredient's unit, after whatever title the row has (an Ingredient's name, a Stock Item's Movements link). |
| `IngredientRows(ingredients)` | A list of a Recipe's or a Prep Session's Ingredients, or of a Plan's freezer splits at put-away, each its name and quantity in its unit, an Ingredient not yet saved with a `New` Tag. |
| `IngredientRowsWith(ingredients, changed, meta)` | `IngredientRows` with each at an index of `changed` marked `Changed` (a Plan Proposal's freezer splits it changes), and what `meta` gives of an Ingredient, unless nil, as its row's `row-meta`: a planner's Recipe Draft's new Ingredients with their proposed values, through `Amount`. |
| `Amount(a, floor, unit)` | Every nutritional value a page shows: the number, then `unit` unless it is `""`; a dash with `sr-only` "unknown" when it isn't known; "29+" when only a floor is known, which a screen reader says "at least 29". `AmountText` is the same as plain text, for a sentence. Never write "unknown" or "at least" for a value by hand. |
| `Seal()` | The pixel sun before an urgent `Row`'s `row-title`. Decorative: the row's text (an accent `Tag`) says why it is urgent. |
| `EditRow(open, summary)` | A line that opens in place (`<details>`) to edit, raised on the surface while open: `summary` is the line, children the edit form. Pass `open` when the form was rejected, so its errors show. |
| `FoldRow(summary)` | A `Row` that opens in place to read (a Prep Session, opening to its Ingredients and Portions): its summary is a Row's `row-title`, `row-figure` and `row-meta`, with a chevron that turns. Unlike an `EditRow` it changes nothing, so it says no "Change" and isn't raised. |
| `EditRowIn(group, open, summary)` | An `EditRow` one of a group, such as the Slots of a day: opening it closes the group's open one. |
| `Field(label, hint, err)` | Any labelled control: the control is the children; `hint` follows the label in brackets (`optional`, a unit); `err` shows under it. A field that may be left blank says `optional`, and a value's unit (`g`, `kcal`, `minutes`) is always in the hint, never in the label or a paragraph; a paragraph says only what a group of values is per (`Per Portion.`). The label sits 4px (`--space-1`) above its own field. |
| `FieldInfo(label, info, hint, err)` | A `Field` with a small information mark; its wrapping tooltip appears on hover, keyboard focus or tap. |
| `FieldInfoCheck(label, info, err)` | A checkbox with its information mark, label and control aligned on one line. |
| `SearchField(label)` | The search box over a list, the input the child: a square box with the magnifying glass inside, so it doesn't read as one of the rows. `label` is for a screen reader; the input's placeholder says it on screen. |
| `ListSearch(id, action, listURL, listID, label)` | A `SearchField` in a form that swaps the list as the User types and opens `action` searched without htmx, the input the child. |
| `TimeField(label, name, value, err)` | A `Field` for a time of day (a Slot time, a Delivery window), typed in either 24- or 12-hour form and shown in the Language's (`web.TimeOfDay`); never a native `type="time"`, which follows the browser's locale. |
| `DateField(label, hint, name, value, err)` | A `Field` for a day, shown in the Language's order (`domain.FormatDate`); never a native `type="date"`. `hint` follows the label: a day that may be left blank says `optional`, and isn't `required`. |
| `CellField(label, err)` | A `Field` in a grid whose column heads name it; the label shows only on a phone. |
| `Check(label, hint)` | A checkbox, the child, before its label (marking an Ingredient a Staple); the whole line is its target. |
| `Checks(legend, hint, name, choices, ticked, err)` | Ticking any of a fixed list of kinds (the kinds of Slot or of appliance a Recipe takes): one `Check` per `Choice`, in as many columns as the width allows, each with its icon when the `Choice` names one, as every kind does. `web.SlotKindChoices` and `web.ApplianceKindChoices` give the lists. |
| `FieldError(err)` | An error outside a `Field`, e.g. for a whole list. |
| `FieldRow()` | A few short `Field`s side by side, wrapping on a phone. Their controls line up even when one label wraps onto two lines. |
| `Actions()` | The row of buttons ending a form; the primary one first. |
| `ActionsApart()` | An `Actions` with one more action that isn't part of the form, at the far end of the row, always in the same place (the Count form's `Movements`). |
| `Button(attrs)` | The primary action of a form, in navy, dithering with slate on hover. One per form. On a page of forms each saved on its own, as the Profile's sections are, each shows Save and has an accessible name that says what it saves (`aria-label="Save Targets"`), so none reads as saving the page and a screen reader tells them apart. The name starts with what the button shows, so voice control's "click Save" still finds it. |
| `TextButton(attrs)` | A secondary action, underlined text. Its target is wider than its text, which, alone, still lines up with the column; beside another, the two keep their targets apart. With `disabled` and `aria-disabled="true"` it is an action shown but unavailable for now (Today's put-away Count and Move before the Delivery arrives): soft ink, no underline, standing where its link will. |
| `TextButtonFold(label, open)` | A secondary action that needs something first, opening in place with a chevron (Reject, opening to the earlier Planning Notes, a hint and the note, and its own button). In `Actions` beside a `Button`, it opens onto a line of its own. Closed unless it holds what the User is at, such as a refused form. |
| `ButtonFold(label, open)` | A `TextButtonFold` that is the page's step forward, so a filled `Button` with its chevron (Plan again after a failed planning Run, opening to the hint, the Planning Note and its own button). |
| `TextLink(attrs)` | A secondary action that writes nothing, looking like a `TextButton`. Unlike one in a form, it isn't what Enter presses. It may lead with an `Icon` when it is a page's way to a page of its own (the Cycle page's Grocery List), the icon on the middle of its words' lowercase letters. |
| `DestructiveButton(attrs)` | Remove, delete, discard, cancel: vermilion text. Deleting or discarding what the User wrote asks once, naming what will go; nothing is removed by a single tap. Its Delete or Discard is a `RemoveLink` to a `RemoveStep`, whose own `DestructiveButton` does it. Cancelling a Run stays one tap: the User didn't write it. |
| `RemoveLink(id, href, src)` | A `DestructiveButton`'s look on a `SideSheetLink`: opens the `SideSheet` `id` on the `RemoveStep` at `src`, or without htmx goes to `href`, the step as a page of its own. |
| `RemoveStep(question, remove, action, back, attrs)` | The step that asks once: `question` names what will go ("Delete the Recipe Rice bowl?"), and its form posts to `action` with a `DestructiveButton` saying `remove`; children are its hidden fields, `attrs` the htmx swap of a sheet's body if any. In a sheet, the sheet's Close is the way back; as a page, `back` gives a `TextLink`. |
| `Segmented(legend, name, choices, selected, err)` | Choosing one of two to four options at a glance (moving a Portion), the chosen one solid navy. `legend` shows over them as a `Field`'s label; leave it empty when the group around it already names them (the Profile's Language). For more, or in a long form, use a `<select>`. |
| `SegmentedLinks(label)` | A `Segmented` whose options each open something instead of filling in a form, so what is shown is reachable from the others (a Stock Item's Movements in each Storage Location). Children are `SegmentLink`s; `label` is for a screen reader. |
| `SegmentLink(current, attrs)` | One option of `SegmentedLinks`, marked `aria-current` and shown as the chosen segment when `current`. `attrs` give its `href`, and in a sheet the swap of its body. |
| `Alert()` | Something went wrong: a rejected form, a failure. On the salmon wash, with a vermilion edge. |
| `ErrorSummary(refusals)` | The top of a rejected form longer than a screen (see [Refusals](#refusals)): an `Alert` linking to each `web.Refusal`'s control by its id, taking focus as the page loads. A refused `Checks` or `Segmented` gives its first box its `name` as id to link to. |
| Floating Notice | Something happened as asked ("Profile saved."), floating at the top of the screen: `web.RedirectWithNotice` for a plain post, `web.FragmentWithNotice` for an htmx answer, `web.WithNotice` for a page. `Layout` shows it; see [Notices](#notices). There is no in-flow notice: one in the page or sheet it confirms pushed the form down, and the list changing behind a sheet may be off-screen. |
| `ToneTag(tone)` | A `Tag` with a tone of its own: `going` in navy (a Run going, a Cycle being planned), `done` in sage (a succeeded Run). A Run's status uses these, `Tag(accent)` for failed and cream for the rest. |
| `ButtonLink(attrs)` | A `Button` that is a link: a state's one step forward when it only goes to another page (Open a Cycle, Plan the Cycle). Dropping sits apart after it, in `ActionsApart`. |
| `Changed()` | What a Plan Proposal changes of the Plan it replaces, worth weighing: a cream `Tag` with the `changed` icon (•) and its word for a screen reader, after a changed day's, Slot's, Prep Session's or freezer split's title. |
| `Tag(accent, title)` | A short label on a row (`New`, a Run's status), a flat cream fill. `accent` when it is wrong or urgent (a failed Run, an urgent row): vermilion with paper text. |
| `AddButton(choices)` | A page's primary add action, one per page: a square vermilion `+` opening the sheet of the first `AddChoice`. One choice is a plain `+`. With more, the `+` shows the icon of what it adds, and a chevron joined to it opens a menu (a native popover of radios, so it needs no JS) that chooses another: choosing changes what the `+` adds and opens nothing, so the `+` always says what it will do. The first choice is the most frequent, and a new page starts on it again. An `AddChoice` whose sheet doesn't hold its body gives `Src` and `Href`, as a `SideSheetLink` does. One with only `Href` is a link to a page, for a form too long for a sheet. A page leads with what the User comes to find, its list; its add forms open from the `+` (paste and write on the Recipe Library), not above it. |
| `AddSheet(id, title, open)` | The sheet of an add form: from the bottom on a phone, centred on desktop, over a navy dithered veil. Every add form is one, whichever choice of the `AddButton` opens it. Every sheet is modal, a `<dialog>` opened with `showModal()`: the page behind it is inert and holds still, Tab stays in it, and a tap on the veil or Esc closes it, the top one where one sheet is open over another. A sheet taller than the screen scrolls itself, and only itself. Once the User has typed or chosen in its form since it opened, or since its last save for a sheet that keeps working (the Count), a tap on the veil leaves it open with what they typed; typing in a search box doesn't count. Esc and Close still close it, discarding the changes. Pass `open` when the form was rejected: it reopens with focus on the first field marked `aria-invalid`. |
| `SideSheet(id, title)` | A sheet of content any page can open without leaving it (the Ingredients): from the right, the screen's full height and up to 60% of its width, on desktop; from the bottom on a phone, as `AddSheet`'s; over the same navy dithered veil, and modal as every sheet is, keeping a changed form on a tap on the veil (see `AddSheet`). Its body is empty until opened. A page renders it once, then opens it with `SideSheetLink`s. |
| `SheetBody(id)` | The body of a sheet whose content swaps in place: every `SideSheet`'s, and an `AddSheet`'s that keeps working through several saves, as the Kitchen's Count does, or that loads as it opens, as Add Portions does. A form in it swaps it (`hx-target` on `SideSheetBody(id)`); a rejected one comes back with focus on its first field marked `aria-invalid`, or else the one marked `autofocus`, the sheet staying open. A swap into part of the body, such as a search's results, leaves the focus where it is. |
| `autofocus` | Only for a sheet's first field as the sheet opens, or a page whose one job is that field: the Count page's search, a Count's or a Move's quantity. A page of a longer form, such as Add Portions without htmx, focuses nothing: on a phone the keyboard would cover half the page, and a screen reader would start past the `h1`. |
| `SectionBody(id)` | A section of a page whose form swaps it in place, the form the children: each of the Profile's sections, saved on its own. The form targets `#id` with `innerHTML`; a rejected one comes back with focus on its first field marked `aria-invalid`. |
| `SideSheetLink(id, href, src)` | Opens the sheet `id`, loading its `SheetBody` from `src`, a fragment, and focuses its field marked `autofocus`, or else its first. Without htmx it is a plain link to `href`, a page of the same content. Placed inside another sheet, it opens over that sheet, which stays open beneath it. |
| `SideSheetLinkWith(id, href, src, attrs)` | A `SideSheetLink` with more attributes, such as an `hx-include` adding to `src` what the page around it says at that moment. |
| `LanguageSwitch(u)` | The pages before a User has chosen their Language (sign-in, the Language choice): a link to each Language, named in itself, staying at `u`. The choice is remembered in this browser. |
| `Markdown(text)` | Text an agent wrote in Markdown, such as a Run's notes. Raw HTML in it is left out. |
| `Verbatim(text)` | Text shown exactly as it was, its lines kept, on the surface: what the User pasted (on a Run and its Recipe Draft), a Trace Event's data. |
| `Icon(name)` | A 10×10 pixel icon, inline SVG in the text colour. Draw new ones in `icons.go`; an unknown name fails the render. |

Two layout classes go on elements a component can't wrap, such as a
`<form>`: `stack` spaces its children evenly in a column (a form's fields and
alert), and `section` gives an unnumbered group a `Section`'s spacing.

Base elements (`input`, `select`, `textarea`, `table`, `dl`, `h1`–`h3`, lists)
are styled once in `app.css`: every text input, select and textarea a square
1px navy box (`--radius` 0), a refused one's box thicker and vermilion, the
dense grids' cells included; tables ruled like rows, `dl` as a two-column list
of facts.

Five feature-shaped components live in `app.css` because nothing else needs
them yet: the Profile's `appliance-grid`, each kind's `Check` with the note on
its model beside it under column heads (`appliance-grid-head`), or under it
on a phone, where the heads are hidden and the note's placeholder, "Model
(optional)", says on screen what it is, as a `SearchField`'s does, a hidden
label naming it for a screen reader at every width; the Recipe form (`ingredient-grid`, `ingredient-row`, `ingredient-details`, `draft-steps`), and the
Run `trace` panel, and `cycle-switcher`, which keeps the current and upcoming
Cycle links compact on a page and stacks them into full-width touch targets on
a phone.

## Accessibility baseline

- Text and controls meet WCAG AA contrast. On paper: ink 17:1, soft ink
  6.7:1, accent 5.8:1, navy 12:1. On navy: paper 12:1, the other tabs'
  `#b9bdd3` 6.6:1, and paper on slate (the lighter of a control's dither) 7:1.
  Paper on accent 5.8:1, accent on its salmon wash 4.9:1, ink on a cream tag
  14:1.
- Non-text indicators reach 3:1: the current tab's salmon bar is 5.8:1 on
  navy, a section rule, a section's stroke, a field's box and the focus
  ring 12:1 on paper, and a salmon focus ring 5.8:1 on navy.
- Touch targets are at least `--tap` (44px).
- Focus is always visible: a 3px navy ring with a 2px gap of paper on every
  control, inputs included, so a field focused as a sheet opens doesn't look
  refused, and a focused field stands out from an invalid one, which keeps its
  thicker vermilion box and `aria-invalid`. On the navy tabs
  and a floating Notice the ring is salmon (5.8:1), as navy is lost there.
- `prefers-reduced-motion` turns off transitions and animations: the busy
  bar's blocks and a busy button's dither stand still.
  The bar still waits its 300 ms. A floating Notice appears and goes with no
  transition, its line still and whole, and still goes when its time runs out.
- Meaning is never carried by colour alone: an error says what is wrong and
  is marked `!`, an invalid field has a thicker box and
  `aria-invalid`, an urgent row has the pixel seal *and* text, a value worth
  weighing has a shape *and* its word for a screen reader, the current tab
  has a bar, paper text and `aria-current`.
- Icons, the seal and the pixel art are `aria-hidden`; what they mean is said
  in text or an `aria-label`.

## Do and don't

- **Do** build a page from the components above; add a component to
  `internal/web` when a piece will recur, and catalogue it here.
- **Do** use tokens for every colour, space and radius.
- **Do** let space separate things: hairlines and section rules, not boxes.
- **Do** keep dithering to the sky, the veil, a button's hover and a busy
  button, and the banner's colours other than navy to the sky and the pixel
  art.
- **Do** give every form one primary `Button`; everything else is a
  `TextButton` or `DestructiveButton`.
- **Don't** write a colour value outside the token block, or in a template.
- **Don't** restyle base elements for one feature (`.profile input`,
  `.stock-form select`): change the base style or add a component.
- **Don't** use the accent for anything that isn't wrong, urgent or
  irreversible: a soft miss is worth weighing, in cream.
- **Don't** nest a heading under a `Group`, or write a nutritional value
  without `Amount`.
- **Don't** add rounded corners, stepped corners, shadows, smooth gradients or
  a second accent.
- **Don't** add an icon font, a stroke icon or runtime JS for presentation
  (the busy button's and the floating Notices' scripts are the exceptions).
- **Don't** set body text, labels, rows or a title's data in the pixel face.
