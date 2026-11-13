# Grafana

The server's spans and logs go over OTLP to a Grafana Cloud stack in the EU (`internal/telemetry`). The `grafana` MCP server in `.mcp.json` is Grafana's hosted one: it acts as the user, signed in through `/mcp` with OAuth, under their permissions. The consent page asks for read, query and write access separately; dashboards need write. Investigating something goes through the `grafana-debugging` skill, which holds the queries; this file holds the facts they stand on.

## Where it is

| | |
|---|---|
| Spans | Tempo, datasource UID `grafanacloud-traces`. |
| Logs | Loki, datasource UID `grafanacloud-logs`. The server's lines have only the labels `service_name` and `deployment_environment_name`; the browser's are below. Everything else on a line is structured metadata, its dots turned to underscores (`trace_id`, `dandori_user_id`, `detected_level`), filtered after the selector: `{service_name="dandori", deployment_environment_name="prod"} \| trace_id="<id>"`. |
| Browser | Frontend Observability's `dandori` app (Faro, ADR 0020), `frontend_observability_handler`: `list` gives its `appId`, 8291, then `errors` and `web-vitals` (with `rangeHours`, at most 168, and `pageId` for one page, as `page_id` names it: `/recipes`, `/runs/*`). What it reports lands in Loki, `grafanacloud-logs`, as below. |
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
| Beat spans | `kitchen beat`, every five minutes on the clock, with its queries under it. A beat that fails for a User logs the error; its span stays OK. |
| Run spans | `run <kind>` (`run parse`, …), under the request that started the Run though it outlives it, ending when the Run ends: the queries its tools run and its calls to the model are under it. Only a Run that failed is an error span, its status the Failure's reason (`model_refused`, `model_failed`, …). |
| Model call spans | client spans under the Run's, named `chat <model>`, one for each call, a retried one too: their duration is the call's latency. A call that failed is an error span with the cause; one cut off by a cancel or a shutdown is not. |
| Logs | `serving` at startup, and errors (the error in `exception_message`): every 5xx logs one, and so does a failed beat or render. They carry `trace_id` and `span_id` when logged in a span, and `dandori_user_id` when logged in a signed-in User's request. |

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

Only production's pages send it: they load Faro when `DANDORI_FARO_URL` is set, and the Faro app takes reports from the production origin only, so there is no `dev` to filter out. A browser with a privacy blocker sends nothing: EasyPrivacy, which Brave Shields and uBlock Origin use by default, blocks Faro's collector, and Dandori respects it (ADR 0020). No reports from a User is no evidence that nothing went wrong. Each report comes from the `dandori` app at its `version`, the commit, as on the server, and carries:

| | |
|---|---|
| User | `dandori.user.id`'s value as Faro's user id, absent before sign-in. |
| Page | Its path, never its query string, with two attributes: `lang`, `<html>`'s as the report was sent (the Language the page spoke, or the one a translator rewrote it to), and `translated`, `"true"` once Chrome's translator marked `<html>` (`translated-ltr` or `translated-rtl`). Edge's translator leaves no mark. |
| Session | Faro's own random id, kept in the tab's session storage, which groups one visit's reports. |
| Browser | Faro's defaults: the browser and OS, their versions, the user agent, the viewport and `navigator.language`. |

What it reports:

| Kind | |
|---|---|
| `exception` | A JavaScript error nothing caught, or a rejected promise nothing handled, with its stack. |
| `measurement` | Web vitals: LCP, CLS, INP, FCP, TTFB. Boosted links never unload the page, so LCP, FCP and TTFB are only the first page's, and CLS and INP span every page of the visit, reported as it ends under the path then current: a page's own vitals are only those of a visit that started on it. |
| `event` `session_start`, `session_resume` | A new Faro session, or one picked up again on a page load. |
| `event` `page translated` | The browser translated the page, with the `lang` it went to. |
| `event` `htmx request` | Each request htmx made, with `method`, `path`, `status` and `trace_id`, the request span's: the link from the browser to the server's spans. Sent before the swap, so its page and `lang` are those of the page the request left from. |

Not the console, nor form contents, nor resource timings, nor clicks.

In Loki each report is a logfmt line under the labels `service_name="dandori"` (the server's own), `app_id="8291"`, `app_key` and `kind`, with no `deployment_environment_name`: a selector with `app_id` gets only the browser's lines, one with `deployment_environment_name` only the server's. Its fields, parsed with `| logfmt` (which names the line's own `kind`, a label too, `kind_extracted`):

| Field | |
|---|---|
| `user_id`, `session_id` | The User and the Faro session. |
| `page_url`, `page_id` | The page's URL, without its query string, and its route as Frontend Observability groups pages (`/runs/*`). |
| `page_attr_lang`, `page_attr_translated` | The page attributes above. |
| `event_name`, `event_data_<name>` | An event and its attributes: `event_data_trace_id`, `event_data_path`, … |
| `app_version` | The commit. |

### Privacy

A User is identified by their internal id only: never their email, a token, the API key or what they typed into a form, in a span, a log line, the `X-Trace-Id` header or what the browser sends Faro, whose URLs go without their query string. The client IP `otelhttp` records stays while Dandori is a small private beta. Anything added to the telemetry keeps to this.

Grafana calls a span's tree a trace; in this repo's code and docs it is spans, since a Run's Trace is a domain term (`GLOSSARY.md`).

## Dashboards and alerts

A dashboard or an alert lives in the owner's Grafana Cloud stack, beside whatever they built themselves: show them what it will hold (panels and their queries, a rule's query and condition) and get a yes before creating or changing one. Put Dandori's in the `Dandori` folder (UID `dandori`), and give each dashboard an environment variable over `deployment.environment.name`, defaulting to `prod`. Create through the provisioning API with `X-Disable-Provenance: true`, so the owner can still edit it in the UI.

| | |
|---|---|
| Dashboard | `Dandori`, UID `dandori-overview`, variable `env` (`prod`, `dev`). Panels: requests, p95 latency and 5xx per route (TraceQL metrics over the request spans; the p95 is Tempo's histogram bucket, a power of two), Supabase failures (its error spans, by call), the beat (`kitchen beat` spans beside the beat's error lines) and the server's error lines (Loki). |
| `Dandori is down` | UID `dandori-down`. Fires when the Synthetic Monitoring check `dandori-healthz` (GET `https://dandori.simopzz.com/healthz` from Frankfurt and Paris every 2 minutes) has failed from both probes for 5 minutes: `max(probe_success{job="dandori-healthz"}) < 1` in `grafanacloud-prom`. No data, the check gone, notifies too. |
| `Dandori server errors` | UID `dandori-errors`. Fires at 3 or more of prod's error lines in 5 minutes, `sum(count_over_time({service_name="dandori", deployment_environment_name="prod"} \| detected_level="error" [5m])) > 2` in `grafanacloud-logs`. Tempo can't back an alert, so it counts log lines, not spans: a failed beat or render counts too. No data means none, and is OK. |
| Contact point | `Dandori owner`, email to the owner, set on each rule (the stack's root notification policy is untouched). |

Synthetic Monitoring checks are made through `run_gcx_command` (`synthetic-monitoring checks list`, `get`, `status`); its `create` and `update` read a YAML file the sandbox doesn't have, so a new check is made in the UI, under Testing & synthetics → Synthetics. Its warning "Alerting setup is incomplete" is about Synthetics' own per-check alerts, which Dandori doesn't use: `Dandori is down` alerts on the check's `probe_success`.

Calls through this server count toward the stack's Grafana Assistant usage; one that asks the Assistant (`ask_assistant`, investigations) spends its tokens.
