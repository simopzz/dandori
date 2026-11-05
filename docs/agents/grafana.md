# Grafana

The server's spans and logs go over OTLP to a Grafana Cloud stack in the EU (`internal/telemetry`). The `grafana` MCP server in `.mcp.json` is Grafana's hosted one: it acts as the user, signed in through `/mcp` with OAuth, under their permissions. The consent page asks for read, query and write access separately; dashboards need write.

## What's there

| | |
|---|---|
| Service | `resource.service.name = "dandori"` |
| Where from | `resource.deployment.environment.name`: `"prod"` (the server) or `"dev"` (`mise run dev`). Filter on it in every query and panel. |
| Request spans | server spans named for the route, e.g. `GET /profile`, `GET /{$}` for Home; `http.route` and the status code as attributes. `/healthz` has none. |
| Query spans | client spans under the request or beat, named for the SQL's operation (`SELECT`, `BEGIN`, …), with the SQL but never its arguments. |
| Supabase spans | client spans under the request that called Supabase Auth, named for the call: `supabase fetch JWKS` (a fresh server's, or a key it hasn't seen, at most once a minute), `supabase renew session`, `supabase send magic link`, `supabase verify link`, `supabase sign out`. The URL and status code as attributes, never a token, the API key or an email. A 4xx Supabase refused is an error span. |
| Beat spans | `kitchen beat`, every five minutes on the clock, with its queries under it. |
| Logs | Loki, carrying `trace_id` and `span_id` when logged in a span. Few for now: `serving` at startup, and errors. Find the labels with `list_loki_label_names` rather than guessing them. |

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
