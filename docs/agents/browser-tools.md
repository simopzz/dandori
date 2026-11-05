# Browser tools

Two MCP servers drive a browser. Pick by the question you're asking:

| You want to | Use |
|---|---|
| Click through a page, take a screenshot, check a change looks right in the user's own Brave | `playwright-brave` (user scope, not in this repo): it asks the user which tab to take over |
| Find out *why*: a token from `app.css` that doesn't apply, a slow page or swap, an accessibility failure, a leak | `chrome-devtools` (`.mcp.json`): DevTools in a Chromium of its own |

## chrome-devtools

It launches `/usr/bin/chromium` on its first tool call (`new_page`, `navigate_page`), not before, with a profile kept in `~/.cache/chrome-devtools-mcp/chrome-profile`, so a sign-in survives between sessions. Run the app with `mise run dev` (http://127.0.0.1:8080). The first time, signing in goes through the real Supabase Auth: ask the user to finish the magic link in the window it opened.

Which tool answers which question:

- **A style that doesn't apply**: `get_css_styles` on the element's `uid` (from `take_snapshot`). It returns the cascade: every matching rule, which declaration wins, and what's inherited. Check it against the tokens in `docs/design.md` before changing CSS.
- **Accessibility, SEO, best practices**: `lighthouse_audit`, either in `navigation` mode (it reloads the page) or in `snapshot` mode, which audits the page as it is now, e.g. after an htmx swap.
- **Speed**: `performance_start_trace` with `reload` for a page load, or start it, do the interaction, then `performance_stop_trace` for a swap. Drill into an insight (`LCPBreakdown`, `INPBreakdown`, `DocumentLatency`) with `performance_analyze_insight`. To see what a slow phone gets, set `emulate` with `cpuThrottlingRate: 4` and `networkConditions: "Slow 4G"` first. `emulate` replaces every earlier setting, so pass all of them in one call.
- **A request or console error**: `list_network_requests` / `get_network_request` (full headers and bodies) and `list_console_messages` with `includeStackTraces`.
- **Leaks**: `take_heapsnapshot` before and after repeating the suspect swap. Comparing the two snapshots and finding what retains an object needs the server started with `--memory-debugging`. Add it to `.mcp.json` locally for that session.

Trace, heap and Lighthouse files go to the OS temp directory unless you give a `filePath` there.
