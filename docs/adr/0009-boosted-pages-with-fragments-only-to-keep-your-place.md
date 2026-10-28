# Boosted pages, with fragments only to keep your place

Every link and form rides an `hx-boost` on `<body>`: htmx fetches the page and swaps the body, so the handlers answer htmx and a plain browser the same way (redirect after a write, the page re-rendered with 422 on a rejection) and none of them branch on `HX-Request`. A boosted swap scrolls to the top, which is right for moving between pages and wrong for acting on one row of a long list. Only there, such as a Stock item halfway down the Stock page, does a form swap a fragment of its own, so the user keeps their place and sees an error beside the row it is about.

## Considered Options

- **A fragment per form** (how the Profile form started): each form needs a fragment template, an `HX-Request` branch in its handler and a test for each path, mostly to show what a full page would have shown.
- **Plain full-page loads**: no htmx code at all, but every write reloads the page and loses the user's place in a list.

## Consequences

- A rejection (422) keeps the address of the page the form was on (`hx-status:422="push:false"`), since a form's action may answer only a POST: pushing it would make reload and Back fail.
- htmx's settling is off (`defaultSettleDelay:0`). Settling briefly gives a swapped-in element the attributes of the old one with the same id, so a script in the new page, such as the one reopening a rejected add form's sheet, would see the old page.
- Scripts in a page are re-run by each boosted swap and nothing is unloaded, so anything long-lived, such as the live Trace's stream, comes from an htmx extension that cleans up when its element is removed, not from a hand-written script.
