# Faro, a third-party script on every page

In production every page loads Grafana's Faro Web SDK, a 126 KB script that Grafana builds, for what only the browser sees: JavaScript errors, web vitals, whether the browser translated the page, and in which `lang`. An investigation into a browser-translated page (2026-11-12) found the browser but not the cause, because the server can't see what happens after a page arrives. Faro reports to the same Grafana Cloud stack as the server's spans, on its free plan (50k sessions a month), and Grafana's Frontend Observability app reads it. Until now every script on a page was htmx or our own.

The SDK is vendored into `/static/` like htmx, pinned, with its checksum in `internal/web/web.go`, so no page loads code from a CDN. Only `DANDORI_FARO_URL` turns it on, so `mise run dev` and the tests load nothing, and the Faro app accepts reports only from the production origin. Our own `static/faro.js` starts it with only the errors, web vitals and session instrumentations, never Faro's defaults: those add console logs, user actions, resource timings and more.

## Considered Options

- **Faro's tracing package as well** (+90 KB): the browser's fetch spans become the parents of the server's request spans, one tree of spans from the click to the query. But its fetch spans record full URLs, query strings included; a client that sent `traceparent` would choose whether the server's spans are sampled; and it wraps `window.fetch`, so a fault in it breaks every htmx request. Instead, an `htmx request` event carries the `X-Trace-Id` each response already has, and the server still ignores `traceparent`.
- **Our own error and translation reporting to a route of the server's**: no third-party code, but our own collector, storage and web vitals, built again.

## Consequences

- Upgrading Faro is a reviewed change to the vendored file, as with htmx.
- Every URL leaves the browser without its query string, which can hold what the User typed; a User is identified by their internal id only (`docs/agents/grafana.md`).
- A Content Security Policy, when there is one, must allow the collector's origin in `connect-src`.
