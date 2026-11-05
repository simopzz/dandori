# CLAUDE.md

This repo's instructions for every agent:

@AGENTS.md

## Claude Code

The `chrome-devtools` server's tools are `mcp__chrome-devtools__*`, deferred: load the ones you need with ToolSearch (`select:mcp__chrome-devtools__get_css_styles,...`). If they're missing, the user hasn't approved `.mcp.json` yet: ask them to run `/mcp`. The same holds for the `grafana` server's `mcp__grafana__*`, which `/mcp` also signs in to Grafana Cloud.
