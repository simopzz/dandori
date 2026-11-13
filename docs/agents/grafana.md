# Grafana

The server's spans and logs go over OTLP to a Grafana Cloud stack in the EU (`internal/telemetry`). The `grafana` MCP server in `.mcp.json` is Grafana's hosted one: it acts as the user, signed in through `/mcp` with OAuth, under their permissions. The consent page asks for read, query and write access separately; dashboards need write. Investigating something goes through the `grafana-debugging` skill, which holds the queries; this file holds the facts they stand on.

## Where it is

| | |
|---|---|
| Spans | Tempo, datasource UID `grafanacloud-traces`. |
| Logs | Loki, datasource UID `grafanacloud-logs`. Its only labels are `service_name` and `deployment_environment_name`. Everything else on a line is structured metadata, its dots turned to underscores (`trace_id`, `dandori_user_id`, `detected_level`), filtered after the selector: `{service_name="dandori", deployment_environment_name="prod"} \| trace_id="<id>"`. |
| Browser | Frontend Observability's `dandori` app (Faro, ADR 0020), `frontend_observability_handler`: `list` gives its `appId`, then `errors` and `web-vitals` (with `rangeHours`, at most 168, and `pageId` for one path). What it reports lands in Loki, `grafanacloud-logs`, as below. |
| Retention | 14 days for spans and logs, the free plan's, with no archive. A Run's own Trace stays in the database (`trace_event`). |

The tools name their arguments differently:

- Tempo: `datasourceUid`; `search_tempo_traces` and `query_tempo_metrics` take `query`, `start`, `end` (RFC3339 with an offset); `get_tempo_trace` takes `trace_id`, and `start` with `end` (both or neither) to make it cheaper.
- Loki: `datasourceUid`, `logql`, `startRfc3339`, `endRfc3339` (which also take `now-6h`).
- With no time range every one of them, the label and attribute lookups too, covers only the last hour.
- Tempo's search lags a minute or two behind: a request made just now is found by its trace id with `get_tempo_trace` before any TraceQL finds it.

## What's there

| | |
|---|---|
| Service | `resource.service.name = "dandori"`, at `resource.service.version`: the commit the image was built from (`dev` for a local build). |
| Where from | `resource.deployment.environment.name`: `"prod"` (the server) or `"dev"` (`mise run dev`). Filter on it in every query and panel. |
| Request spans | server spans named for the route, e.g. `GET /profile`, `GET /{$}` for Home; `http.route`, the status code, the client IP and the attributes below. `/healthz`, `/static/*`, `/sw.js` and `/manifest.webmanifest` have none. Only a 5xx is an error span. Every response carries its span's trace id in `X-Trace-Id`, and the 500 page shows it: search Tempo for the id a report quotes. |
| Query spans | client spans under the request or beat, named for the SQL's operation (`SELECT`, `BEGIN`, …), with the SQL but never its arguments. |
| Supabase spans | client spans under the request that called Supabase Auth, named for the call: `supabase fetch JWKS` (a fresh server's, or a key it hasn't seen, at most once a minute), `supabase renew session`, `supabase send magic link`, `supabase verify link`, `supabase sign out`. The URL and status code as attributes, never a token, the API key or an email. A 4xx Supabase refused is an error span. |
| Beat spans | `kitchen beat`, every five minutes on the clock, with its queries under it. |
| Run spans | `run <kind>` (`run parse`, …), under the request that started the Run though it outlives it, ending when the Run ends: the queries its tools run and its calls to the model are under it. Only a Run that failed is an error span, its status the Failure's reason (`model_refused`, `model_failed`, …). |
| Model call spans | client spans under the Run's, named `chat <model>`, one for each call, a retried one too: their duration is the call's latency. A call that failed is an error span with the cause; one cut off by a cancel or a shutdown is not. |
| Logs | `serving` at startup, and errors (the error in `exception_message`), carrying `trace_id` and `span_id` when logged in a span, and `dandori_user_id` when logged in a signed-in User's request. |

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

### Run and model call span attributes

| Attribute | |
|---|---|
| `dandori.run.id` | The Run's id, an integer, as `run.id` in the database and in its page's path, `/runs/<id>`. On the Run's span and on each of its calls'. |
| `dandori.run.kind`, `dandori.user.id` | On the Run's span: its kind (`parse`, …) and whose it is. |
| `dandori.run.status` | On the Run's span: how it ended, `succeeded`, `failed` or `cancelled`. |
| `gen_ai.operation.name`, `gen_ai.request.model` | `chat`, and the model asked. |
| `gen_ai.provider.name`, `gen_ai.response.model` | `anthropic`, and the model that answered: another than the one asked after a server-side fallback. Absent with the demo model. |
| `gen_ai.response.finish_reasons` | Why the model stopped, as the Trace's stop reasons: `["tool_use"]`, `["end_turn"]`, `["max_tokens"]`, `["refusal"]`. |
| `gen_ai.usage.input_tokens` | Every input token, the cache's included, as OTel's conventions count them (the API's own `input_tokens` leaves the cache out). |
| `gen_ai.usage.cache_read.input_tokens`, `gen_ai.usage.cache_creation.input_tokens` | The input tokens read from and written to the prompt cache, which are priced differently from uncached ones. |
| `gen_ai.usage.output_tokens`, `gen_ai.usage.reasoning.output_tokens` | The output tokens, thinking included, and the thinking alone. The demo model reports every count as 0. |
| `error.type` | On a call that failed: the API's status code (`429`, `529`, `400`, …), `timeout`, or the Go error's type for a network failure (`*url.Error`). The status description has the cause in full. |

No span holds what the model was asked or said, nor the API key: that is the Run's Trace, in the database.

### Browser telemetry

Only production's pages send it: they load Faro when `DANDORI_FARO_URL` is set, and the Faro app takes reports from the production origin only, so there is no `dev` to filter out. Each report comes from the `dandori` app at its `version`, the commit, as on the server, and carries:

| | |
|---|---|
| User | `dandori.user.id`'s value as Faro's user id, absent before sign-in. |
| Page | Its path, never its query string, with two attributes: `lang`, `<html>`'s as the report was sent (the Language the page spoke, or the one a translator rewrote it to), and `translated`, `"true"` once Chrome's translator marked `<html>` (`translated-ltr` or `translated-rtl`). Edge's translator leaves no mark. |
| Session | Faro's own random id, kept in the tab's session storage, which groups one visit's reports. |

What it reports:

| Kind | |
|---|---|
| `exception` | A JavaScript error nothing caught, or a rejected promise nothing handled, with its stack. |
| `measurement` | Web vitals: LCP, CLS, INP, FCP, TTFB. |
| `event` `session_start` | A new Faro session. |
| `event` `page translated` | The browser translated the page, with the `lang` it went to. |
| `event` `htmx request` | Each request htmx made, with `method`, `path`, `status` and `trace_id`, the request span's: the link from the browser to the server's spans. |

Not the console, nor form contents, nor resource timings, nor clicks.

In Loki, Faro's convention is a logfmt line under the labels `app_name="dandori"` and `kind`, with the event's attributes as `event_data_<name>` (`event_data_trace_id`) and the page's as `page_attr_<name>`. No Faro app had reported when this was written: check the labels with `list_loki_label_names` on the first data and correct this paragraph.

### Privacy

A User is identified by their internal id only: never their email, a token, the API key or what they typed into a form, in a span, a log line, the `X-Trace-Id` header or what the browser sends Faro, whose URLs go without their query string. The client IP `otelhttp` records stays while Dandori is a small private beta. Anything added to the telemetry keeps to this.

Grafana calls a span's tree a trace; in this repo's code and docs it is spans, since a Run's Trace is a domain term (`GLOSSARY.md`).

## Dashboards

A dashboard lives in the user's Grafana Cloud stack, beside whatever they built themselves: show them what it will hold (panels and their queries) and get a yes before creating or changing one. Put Dandori's dashboards in a `Dandori` folder, and give each an environment variable over `deployment.environment.name`, defaulting to `prod`.

Calls through this server count toward the stack's Grafana Assistant usage; one that asks the Assistant (`ask_assistant`, investigations) spends its tokens.
