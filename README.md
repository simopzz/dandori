# Dandori

*Dandori* (段取り) is the Japanese art of arranging a task so it runs smoothly. This is a personal meal-planning co-pilot for one person cooking for themselves: it plans batch-cooked meals, tracks what is in the kitchen, adapts when the schedule changes, and fills an online supermarket cart.

## Idea

- **Cook once, eat three times.** A dinner yields three portions: one eaten now, one as lunch two days later, one for the freezer.
- **Plan per delivery.** Planning covers one grocery delivery at a time (a Cycle), to keep delivery costs down.
- **The agent proposes, you approve.** Changes to the plan, such as a meal out, only take effect once you accept them.
- **Nothing is bought for you.** Cart Fill stops at the cart; there is no checkout.
- **See what the agent does.** Every agent run has a live trace of its steps.

The original pitch is in [vision.md](vision.md); the current terminology is in [GLOSSARY.md](GLOSSARY.md) and design decisions are in [docs/adr](docs/adr).

## Status

Early development. So far: the profile (targets, exclusions, meal-out estimate), the agent loop and the live trace panel. Planning, the recipe library and cart fill are still to come.

## Stack

Go, [templ](https://templ.guide) and htmx, PostgreSQL with pgvector (on Supabase), and Claude.

## Running it

Tools are managed with [mise](https://mise.jdx.dev).

```sh
cp .env.example .env   # set DANDORI_DATABASE_URL, and ANTHROPIC_API_KEY if needed
mise run dev           # serves on http://127.0.0.1:8080
```

Other tasks: `mise run build`, `mise run lint`, and `mise run testdb` followed by `mise run test` for the full suite (the tests need a disposable Postgres, started with Docker).
