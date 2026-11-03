# Hosted on a Hetzner VPS with Docker Compose

v2 runs as one binary on a small Hetzner VPS in Germany, behind Caddy, with Docker Compose, deployed by GitHub Actions on every merge to `v2`; its database is a Supabase project in Frankfurt (eu-central-1). Planning Runs and Known Catalog seeds are agent loops of several minutes running in the background, and the scheduler runs in the same process, so the app has to stay up. Serverless containers would have to move them onto a separate jobs product. The User data stays in the EU.

## Considered Options

- **Fly.io in Amsterdam or Frankfurt**: as cheap and nothing to operate, but a US provider.
- **Scaleway Serverless Containers**: EU, likely free, with cron triggers, but long background Runs need redesigning as jobs.
- **Coolify or Dokploy on the same VPS**: a deploy UI for one app, at the cost of another service to run and more memory.
