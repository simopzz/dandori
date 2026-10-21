# Postgres + pgvector on Supabase, not Turso

The app runs locally but its database lives in the cloud. We chose Postgres with pgvector (hosted on Supabase's free tier) over Turso/libSQL: the schema is relational (plans, portions, stock), pgvector powers semantic recipe search, and an existing Supabase account removes setup cost. Turso remains the fallback if a local-first or embedded database is wanted later.

## Consequences

Supabase's free tier pauses projects after a period of inactivity, so the app can find its database paused after a week or more of not being used and must handle that (or the project must be kept active).
