#!/usr/bin/env bash
# Runs the app locally against the dev server: mise run dev, after devdb and generate.
# mise has loaded .env into the environment, so each value below falls back to it.
set -euo pipefail

# The dev server, never .env's DANDORI_DATABASE_URL: a branch's migrations must not
# reach production.
export DANDORI_DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5434/postgres?sslmode=disable"
# Beside another dev server on DANDORI_DEV_ADDR, else on .env's DANDORI_ADDR.
export DANDORI_ADDR="${DANDORI_DEV_ADDR:-${DANDORI_ADDR:-127.0.0.1:8080}}"
# Runs ask the demo model, which costs nothing, unless DANDORI_DEMO_LLM=false is set in
# the shell or .env: then they ask Claude with .env's ANTHROPIC_API_KEY.
export DANDORI_DEMO_LLM="${DANDORI_DEMO_LLM:-true}"
# Supabase's URL and key under either name: DANDORI_ as in .env.example, or bare as
# provision.sh writes them.
export DANDORI_SUPABASE_URL="${DANDORI_SUPABASE_URL:-${SUPABASE_URL:-}}"
export DANDORI_SUPABASE_PUBLISHABLE_KEY="${DANDORI_SUPABASE_PUBLISHABLE_KEY:-${SUPABASE_PUBLISHABLE_KEY:-}}"
# Spans and logs go to Grafana Cloud if .env sets its endpoint, tagged dev to tell them
# from production's.
export OTEL_RESOURCE_ATTRIBUTES="deployment.environment.name=dev"

# A traveled run writes Movements dated in the fake future, and a kitchen never moves
# back: on the dev database they would stay ahead of the next run's clock. So it gets
# dandori_travel_<port>, dropped and made again each start, one per port so two dev
# servers don't share one. A pinned User (email:<uuid>) lives in the dev database.
if [ -n "${DANDORI_DEV_NOW:-}" ]; then
  case "${DANDORI_DEV_USERS:-}" in
    *:*) echo "dev mode on the dev database, for the pinned User: back it up first (docs/agents/dev-mode.md)" ;;
    *)
      travel="dandori_travel_${DANDORI_ADDR##*:}"
      PGOPTIONS="-c client_min_messages=warning" psql "$DANDORI_DATABASE_URL" -q \
        -c "DROP DATABASE IF EXISTS $travel WITH (FORCE)" -c "CREATE DATABASE $travel"
      export DANDORI_DATABASE_URL="postgres://postgres:postgres@127.0.0.1:5434/$travel?sslmode=disable"
      ;;
  esac
fi
exec go run ./cmd/dandori
