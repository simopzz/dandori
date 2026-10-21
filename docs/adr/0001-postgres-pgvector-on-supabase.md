# Postgres + pgvector on Supabase, not Turso

The app runs locally but its database lives in the cloud. We chose Postgres with pgvector (hosted on Supabase's free tier) over Turso/libSQL: the schema is relational (plans, portions, stock), pgvector powers semantic recipe search, and an existing Supabase account removes setup cost. Turso remains the fallback if a local-first or embedded database is wanted later.
