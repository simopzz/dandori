# Hand-rolled agent loop; Go backend is the MCP client for Playwright

The agent uses the Anthropic API with a hand-written tool-use loop rather than a framework, so every step is naturally emitted as a trace event. The Playwright MCP server runs as a local subprocess (it needs the user's logged-in supermarket session) and the Go backend acts as its MCP client.

v2 keeps the hand-written loop but not the MCP client: Cart Fill moved to a browser extension (ADR-0016), which left the client with no caller.
