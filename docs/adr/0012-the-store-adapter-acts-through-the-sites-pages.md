# The store adapter acts through Esselunga's pages, and the model only reads them

Dandori reaches Esselunga through the user's logged-in browser. It could act on the site in two ways: call the site's internal web API, as its own front end does, or use its pages as a person would. It could also let the model browse the site itself. The store port's operations act through the site's pages in fixed code: they navigate, then find and press buttons by their accessible labels, such as "Aggiungi al carrello". The internal API is undocumented, can change without notice, and calling it directly is a use of Esselunga's servers the site never offered. Pages are what the site offers its users, and labels kept for accessibility change less often than layout. Where the model needs to look around the site, for example to find a replacement for a product out of stock, it gets read-only browsing: navigation limited to search and product pages, and a page snapshot. It gets no click and no JavaScript tool, so it can neither change the cart nor reach checkout.

## Considered Options

- **The site's internal API**: precise and fast, and how lookup and order history first worked. It was rejected because the API can change silently, and calling it directly looks like an unauthorized client.
- **The model browses and acts on the site, behind a guard**: it adapts to a changed site by itself. But ADR 0005 would then hold only as long as the guard is right: an address allowlist, refused clicks on checkout-like buttons, and blocked checkout requests. The cart page itself lives under `/checkout/`, a guard matching button wording breaks silently when the site renames one, and a JavaScript tool would get past all of it. It would also cost many more tokens and be less predictable.

## Consequences

- ADR 0005 holds by construction: no store operation checks out, and the model has no tool that could.
- The site's quantity menu ignores a value set from code, and an add that is interrupted fails without an error. So setting a quantity means reading the cart line, deleting it if it holds too many, pressing add once per missing pack with a pause between presses, and then checking the cart page. Setting rather than adding keeps a repeated Cart Fill from doubling a product.
- A change to the site breaks an operation loudly, such as a button that can't be found, rather than doing something else.
- Lookup and order history still call the site's API, until they move to the pages too.
