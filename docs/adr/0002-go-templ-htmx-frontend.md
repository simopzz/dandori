# Go + templ + HTMX frontend, no SPA

The backend is Go and the UI is server-rendered with templ and HTMX, using SSE for the live agent trace and a small vanilla-JS island for Cooking Mode (Wake Lock, timers). This keeps a single deployable binary and avoids a second toolchain; the app is installable as a PWA. A SPA (SvelteKit/React/Next.js) was rejected as unnecessary for a single-user app.
