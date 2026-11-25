# Dev time travel runs the app on a manual clock

`DANDORI_DEV_NOW` runs the dev server on a `clock.Manual` standing at that time, with a fake Supabase Auth and the routes a driver needs under `/_test/`: the clock switch (POST /_test/clock), the mailbox (GET /_test/mail) and the fake Auth itself (/_test/supabase). The time stands still between jumps, and each jump past a beat fires the scheduler's beat once, with the latest, as a machine waking up does. The server wakes up at the dev time when it starts, so what the app assumed up to then is recorded before the first request. `DANDORI_DEV_USERS` invites the emails it lists at boot, each an email or an email and the User's id: with the id, the session is the User of a database built under real Supabase.

The fake Auth is a consequence, not a convenience: Supabase issues its tokens at real time and they last an hour, and the app checks them against the clock it is given (`jwt.WithTimeFunc(s.clock.Now)`), so a dev server traveled past that hour would refuse every real token and could not sign in again, Supabase issuing new ones at real time. A dev server that travels signs Users in with tokens its own clock issued, and the app's auth code is unchanged.

The `/_test/` routes are mounted only when `DANDORI_DEV_NOW` is set, the same operator's contract as `DANDORI_DEMO_LLM`: the env var says what the server is, and the binary stays one binary. Production never mounts them.

## Considered Options

- **Real Supabase while traveled**: impossible by construction, its tokens being issued at real time and expiring an hour after, and the app checking them with the clock it travels.
- **A clock ticking at real speed, fake time flowing**: two clocks' worth of code for nobody's ask; jumping is what a driver needs.
- **The clock switch and the fake Auth mounted always**: production would serve routes that sign Users in with a key it holds; the env var keeps them out.

## Consequences

- A traveled run writes Movements dated in the fake future into the dev database: the dev data is disposable, and part of it goes when a traveled session is over.
- The driver signs in through the app's own pages, with the magic link a User gets: only the Supabase behind it is fake.
- The override is dev-only by design; a production demo, when it comes, is its own decision, not this feature's second half.
- How a driver uses it: `docs/agents/dev-mode.md`.
