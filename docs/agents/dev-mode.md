# Dev mode: the app on a manual clock

`DANDORI_DEV_NOW` runs the dev server on a manual clock at that time, with a fake Supabase Auth and the `/_test/` routes a driver needs (ADR-0022). Reach a state that depends on the date by jumping the clock, not by editing the dev database.

## Starting it

```
DANDORI_DEV_NOW=2026-11-25T08:00 DANDORI_DEV_USERS=ada@example.com mise run dev
```

- **The time**: an RFC 3339 time (`2026-11-25T08:00:00+01:00`) or a local one (`2026-11-25T08:00`), read in the installation's zone (`DANDORI_TIMEZONE`, Europe/Rome) when it carries none.
- **The users**: `DANDORI_DEV_USERS` is a comma list of `email` or `email:<uuid>`. The uuid pins the User: the session is then that User of the dev database, with the data built under real Supabase. Without it the email gets a fresh User with an empty kitchen.
- **The auth**: the fake Supabase Auth replaces the real one, so `DANDORI_SUPABASE_URL` and `DANDORI_SUPABASE_PUBLISHABLE_KEY` are not needed, and the app's auth code is untouched.
- **The model**: `DANDORI_DEMO_LLM=false` (as `.env` has it) asks Claude; the demo model only parses Recipes. A planning Run takes a minute and several turns: watch it on `/runs/<id>`.
- **The database**: the dev database, as `mise run dev` points at it. A traveled run writes into it: Movements dated in the fake future, Runs of the fake future. Back it up and restore it when the session is over (`browser-tools.md` has the `pg_dump` and `pg_restore` commands), or delete the User's rows: `DELETE FROM "user" WHERE email = '<email>'`, the foreign keys cascade.

## The routes under /_test/

| Route | What it does |
|---|---|
| `POST /_test/clock` | Moves the clock: `now=<RFC 3339>`. Answers 204 when the beat is done. |
| `GET /_test/mail?email=<email>` | The fake mailbox: a JSON array of the links sent to that email, oldest first. The last one is the newest. |
| `/_test/supabase/auth/v1/...` | The fake Supabase Auth the app itself signs Users in through. |

They are mounted only when `DANDORI_DEV_NOW` is set: production has none.

## Signing in

The fake Auth emails magic links to invited Users only, into the fake mailbox.

1. `/sign-in`, the email, "Email me a sign-in link".
2. Fetch the mailbox and open its last link: `fetch('/_test/mail?email=ada@example.com')` from the page, same origin, then navigate to the link. It leads to `/auth/confirm`, whose button "Sign in to Dandori" signs you in.
3. The first sign-in lands on the Language choice; later ones land on Today.

The mailbox's links lead to the app as the mailbox was reached, so a browser on `localhost` gets a `localhost` link and keeps its session cookie on that host.

## Moving time

```
curl -X POST --data-urlencode "now=2026-12-01T10:00:00+01:00" http://127.0.0.1:8080/_test/clock
```

- **Encode the form value**: a `+` in the offset becomes a space in a form body and the switch answers 400. Use `Z` (`2026-12-01T09:00:00Z`) or `--data-urlencode`.
- **The clock stands still between jumps**, and each jump past a multiple of five minutes fires the scheduler's beat once, with the latest. The POST answers when the beat is done, so a jump that crosses a Delivery's arrival has already written its Movements.
- **The app's ledger never moves back**: `kitchen.up_to` only moves forward, so a jump backwards leaves the kitchen ahead of the clock. Jump forward.
- **The app's days and Slots follow the clock**: Today shows the day, the Slot now or next, the later ones a click away, and "Kitchen up to date as of <time>".

## What the app does when the clock jumps

- **A Delivery arrives** when the clock passes the end of its window: the Plan page goes from "Tentative" to "Delivered on <day>", and Today shows "Put the Delivery away" with its lines until the end of the day after it. Its Movements are dated at the beat's moment, not at the arrival's, so a jump of an hour dates them an hour late.
- **A Delivery that was never marked ordered still arrives**: the arrival beat does not look at the ordered state. Don't read that as the rule.
- **"Order the next Delivery"** shows while the Delivery is tentative, **"Open the next Cycle"** from three days before the current Cycle's end.
- **v2 writes no assumed Movements yet**: a Prep Session's Ingredients are not used up and a Portion is not prepared or eaten. The kitchen's stock only grows as the plan is eaten, and Today's Slot sections show the day's times, not the dishes — the Plan page shows those. Don't read a traveled kitchen as what the Plan would have used.
- **Today's Plan section is a placeholder**: its empty message ("Your Plan shows here once Dandori plans what you eat…") shows whatever the Plan is.

## Setting a kitchen up

The pages' endpoints take the same posts as their forms, so from the page:

```js
await fetch("/ingredients", {method: "POST", body: new URLSearchParams({name: "rice", unit: "g", usual: "pantry", kcal: "130", protein: "2.7", carbs: "28", fat: "0.3"})});
await fetch("/kitchen/count", {method: "POST", body: new URLSearchParams({ingredient: "17", location: "pantry", stated: "500"})});
```

Drive the form when the form is what you are testing; post when you are only setting up.

**Dates**: a non-Italian Language writes them `mm/dd/yyyy` (`11/02/2026` is 2 November). The form refuses the other order with "Type a date, like 11/02/2026".
