<h1 align="center">
  <img src="assets/banner.png" alt="Dandori 段取り" width="100%">
</h1>

*Dandori* (段取り) is the Japanese art of arranging a task so it runs smoothly. This is a meal-planning co-pilot for a few invited Users, each one person cooking for themselves: it plans batch-cooked meals, tracks what is in the kitchen, adapts when the schedule changes, and fills an online supermarket cart.

## Idea

- **Cook once, eat three times.** A dinner yields three portions: one eaten now, one as lunch two days later, one for the freezer.
- **Plan per delivery.** Planning covers one grocery delivery at a time (a Cycle), to keep delivery costs down.
- **Today tells you what to do.** What to eat next, when to start cooking, what to thaw tonight. What Dandori assumed happened is there to correct.
- **The agent proposes, you approve.** Changes to the plan, such as a meal out, only take effect once you accept them, and every agent run has a live trace of its steps.
- **Nothing is bought for you.** There is no checkout.

What it's for and where it's going is in [vision.md](vision.md); the terminology is in [GLOSSARY.md](GLOSSARY.md) and design decisions are in [docs/adr](docs/adr).

## Status

This is v2, a new codebase (see [ADR-0014](docs/adr/0014-v2-is-a-new-codebase.md)); v1 is frozen on `main`. The walking skeleton is done: an invited User signs in at [dandori.simopzz.com](https://dandori.simopzz.com) by magic link, picks their Language and sees an empty Today, deployed on every merge to `v2`. Next is milestone M1, one real Cycle on v2: Ingredients and the Kitchen Ledger first, then Recipes, the Profile, planning, the Grocery List, the Plan over time and Exceptions.

## Stack

Go, [templ](https://templ.guide) and htmx, PostgreSQL with pgvector (on Supabase), and Claude.

## Running it

Tools are managed with [mise](https://mise.jdx.dev).

```sh
cp .env.example .env   # set the Supabase URL and key, and ANTHROPIC_API_KEY if needed
mise run dev           # serves on http://127.0.0.1:8080
```

`mise run dev` starts a local Postgres in Docker (`mise run devdb`) and runs the app against it, never against `DANDORI_DATABASE_URL`, so a branch's migrations stay off production. Its data is kept between runs; `sudo docker rm -f dandori-devdb && sudo docker volume rm dandori-devdb` starts it over. Signing in still goes through the real Supabase Auth.

Other tasks: `mise run build`, `mise run lint`, and `mise run testdb` followed by `mise run test` for the full suite (the tests need a disposable Postgres, started with Docker).
