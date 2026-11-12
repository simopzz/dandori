---
name: grafana-debugging
description: Use when investigating what Dandori's server did, in prod or a `mise run dev` run: a User's report ("saw X at about 14:00"), a slow page, an error or a quoted trace id, a refused form, a page in the wrong language, a beat.
---

# Debugging through Grafana

Facts this playbook stands on (datasource UIDs, Loki labels, each tool's argument names, every attribute, the privacy rule, retention) live in `docs/agents/grafana.md`. Read it before the first query.

## Before the first query

1. **Pin the window.** Turn the report's time into RFC3339 with the User's offset (Italy is `+01:00` in winter, `+02:00` in summer) and pass a range of an hour or so around it to every call. Without one each tool looks at the last hour only, and an empty result means nothing. "Just now" is whatever `date` says: read it from there, never from earlier in the conversation.
2. **Check retention.** Spans and logs older than 14 days are gone. Say so, and work from what outlives them: the database (a Run's Trace, the ledger's Movements, the User's rows), the deploys (`gh run list --branch v2`) and the code at the commit then running.
3. **Pick the environment**: `prod` for a report, `dev` for your own `mise run dev`.

Every query below is for `prod`; swap the environment for `dev`.

## From symptom to query

### A report quotes a trace id

The `X-Trace-Id` header, or the id the 500 page shows. `get_tempo_trace` with `trace_id`, and `start`/`end` around the report. Its request span carries who, what Language, the status and any refusal; the spans under it are the queries and Supabase calls the request made. Then its log lines:

```
{service_name="dandori", deployment_environment_name="prod"} | trace_id="<id>"
```

### A User saw X

Start from the User's id: the owner looks it up by email in Supabase (Authentication → Users). The email itself stays out of every query. In a quiet window, `list_tempo_attribute_values` on `span.dandori.user.id` lists every User seen, which may be enough. Their requests in the window:

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && kind = server && span.dandori.user.id = "<id>" } | select(span.http.response.status_code, span.dandori.language, span.http.request.header.accept-language, span.dandori.htmx, span.dandori.refusal)
```

`search_tempo_traces` returns only the attributes a query names, hence the `select`. Find the request at the reported time, then open it with `get_tempo_trace`.

Before sign-in there is no id: narrow by route and time instead, `span.http.route = "/sign-in"`.

### A page in the wrong language, or translated

The Language the server rendered against what the browser asked for:

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && kind = server && span.dandori.user.id = "<id>" } | select(span.dandori.language, span.http.request.header.accept-language)
```

The server rendered the Language in `dandori.language`. If it is right and the User still saw another, the browser changed the page after it arrived.

### A slow page

Which routes are slow (`query_tempo_metrics`, `range`):

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && kind = server } | quantile_over_time(duration, .95) by (span.http.route)
```

Then the slow requests of one route, and `get_tempo_trace` on one of them to see whether the time went to queries (`SELECT`, `BEGIN`, … spans) or to Supabase (`supabase …` spans):

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && kind = server && span.http.route = "/recipes" && duration > 500ms }
```

### An error

A 5xx is an error span; a Supabase call it refused is one too:

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && status = error }
```

The logged error, with its message in `exception_message`:

```
{service_name="dandori", deployment_environment_name="prod"} | detected_level="error"
```

### A form refused, a sign-in link that didn't work

A 422's reason is in `dandori.refusal`. Which refusals, how often:

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && span.dandori.refusal != nil } | rate() by (span.dandori.refusal)
```

### The beat

```
{ resource.service.name = "dandori" && resource.deployment.environment.name = "prod" && name = "kitchen beat" }
```

A gap longer than five minutes between them means the server was down or restarting: `serving` in the logs marks each start.

## Asking the Assistant

`ask_assistant` and investigations (`create_investigation`) spend the stack's Assistant tokens. Use them when the owner asks, after the queries above have run out.
