package agent

import (
	"context"
	"encoding/json"
)

// Brief is what the agent is told for one Run and the tools it may use. Each kind of Run
// builds its own.
type Brief struct {
	// System is the system prompt: byte-stable for a kind of Run, so it is cached.
	System string
	// Prompt is the first user turn, holding everything that varies per User or request.
	Prompt string
	Tools  []Tool
	// Effort is how hard the model thinks for this kind of Run, always set explicitly.
	Effort Effort
}

// Tool is something the agent can do. Run's result goes back to the model; an error goes
// back as a failed tool result, so the model can recover.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Run         func(ctx context.Context, input json.RawMessage) (string, error)
}
