package agent

import (
	"context"
	"encoding/json"
)

// Job is the work of one Run: what the agent is told and the tools it may
// use. Each Run kind (planning, Recipe parsing, catalog seed, Cart Fill)
// builds its own.
type Job struct {
	System string
	Prompt string
	Tools  []Tool
}

// Tool is something the agent can do. Run's result goes back to the model;
// an error goes back as a failed tool result, so the model can recover.
// Tools emit their own Trace Events, such as throttle waits, to tr.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Run         func(ctx context.Context, tr Trace, input json.RawMessage) (string, error)
}
