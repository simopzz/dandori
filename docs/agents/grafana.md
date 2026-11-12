# Grafana

The server's spans and logs go over OTLP to a Grafana Cloud stack in the EU (`internal/telemetry`). The `grafana` MCP server in `.mcp.json` is Grafana's hosted one: it acts as the user, signed in through `/mcp` with OAuth, under their permissions. The consent page asks for read, query and write access separately; dashboards need write.

## What's there

| | |
|---|---|
| Service | `resource.service.name = "dandori"`, at `resource.service.version`: the commit the image was built from (`dev` for a local build). |
| Where from | `resource.deployment.environment.name`: `"prod"` (the server) or `"dev"` (`mise run dev`). Filter on it in every query and panel. |
| Request spans | server spans named for the route, e.g. `GET /profile`, `GET /{$}` for Home; `http.route`, the status code, the client IP and the attributes below. `/healthz`, `/static/*`, `/sw.js` and `/manifest.webmanifest` have none. Only a 5xx is an error span. Every response carries its span's trace id in `X-Trace-Id`, and the 500 page shows it: search Tempo for the id a report quotes. |
| Query spans | client spans under the request or beat, named for the SQL's operation (`SELECT`, `BEGIN`, …), with the SQL but never its arguments. |
| Supabase spans | client spans under the request that called Supabase Auth, named for the call: `supabase fetch JWKS` (a fresh server's, or a key it hasn't seen, at most once a minute), `supabase renew session`, `supabase send magic link`, `supabase verify link`, `supabase sign out`. The URL and status code as attributes, never a token, the API key or an email. A 4xx Supabase refused is an error span. |
| Beat spans | `kitchen beat`, every five minutes on the clock, with its queries under it. |
| Logs | Loki, carrying `trace_id` and `span_id` when logged in a span, and `dandori.user.id` when logged in a signed-in User's request. Few for now: `serving` at startup, and errors. Find the labels with `list_loki_label_names` rather than guessing them. |

### Request span attributes

Dandori's own attributes are under `dandori.`; in TraceQL, `span.dandori.user.id` and so on.

| Attribute | |
|---|---|
| `dandori.user.id` | The signed-in User's id: Supabase's user UUID, as `user_id` in the database. Absent when no one is signed in. |
| `dandori.signed_in` | `true` when the route checked the session and found a User; `false` otherwise, including on routes that never check (`/sign-in`, `/auth/confirm`, `POST /sign-out`). |
| `dandori.language` | The Language the page spoke: `en` or `it`. The User's own once signed in and chosen, before that the one the pages were switched to, else `en`. |
| `http.request.header.accept-language` | The browser's `Accept-Language`, as sent. |
| `dandori.htmx`, `dandori.htmx.boosted` | Whether htmx sent the request (`HX-Request`), and from a boosted link or form (`HX-Boosted`). |
| `dandori.refusal` | On a 422 (or a 401 or 403, which nothing answers yet), why, in English: a short reason such as `sign-in link expired or used` or `Language stays: a parse Run is going`, followed by the fields a form refused, as in `Ingredient refused: name, unit`. Never what the User typed. |

```
{ resource.service.name = "dandori" && span.dandori.user.id = "<id>" }
{ resource.service.name = "dandori" && span.dandori.refusal != nil } | rate() by (span.dandori.refusal)
```

### Privacy

A User is identified by their internal id only: never their email, a token, the API key or what they typed into a form, in a span, a log line or the `X-Trace-Id` header. The client IP `otelhttp` records stays while Dandori is a small private beta. Anything added to the telemetry keeps to this.

Grafana calls a span's tree a trace; in this repo's code and docs it is spans, since a Run's Trace is a domain term (`GLOSSARY.md`).

## Querying

TraceQL, through the Tempo data source's tools:

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && kind = server }
{ resource.service.name = "dandori" && name = "kitchen beat" }
{ resource.service.name = "dandori" && kind = client && name =~ "supabase .*" }
```

Request rate and latency per route are TraceQL metrics over the server spans:

```
{ resource.service.name = "dandori" && kind = server } | rate() by (span.http.route)
{ resource.service.name = "dandori" && kind = server } | quantile_over_time(duration, .95) by (span.http.route)
```

## Dashboards

A dashboard lives in the user's Grafana Cloud stack, beside whatever they built themselves: show them what it will hold (panels and their queries) and get a yes before creating or changing one. Put Dandori's dashboards in a `Dandori` folder, and give each an environment variable over `deployment.environment.name`, defaulting to `prod`.

Calls through this server count toward the stack's Grafana Assistant usage; one that asks the Assistant (`ask_assistant`, investigations) spends its tokens.
