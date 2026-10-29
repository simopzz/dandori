package agent

import (
	"context"
	"encoding/json"
)

// Job is the work of one Run: what the agent is told and the tools it may use.
// Each Run kind (planning, Recipe parsing, catalog seed, Cart Fill) builds its own.
type Job struct {
	System string
	Prompt string
	Tools  []Tool
	// Effort is how hard the model thinks for this Job (low, medium, high...).
	// Thinking spends the same token budget as the answer, so a Job whose
	// answer is long keeps it low enough to leave room. Empty leaves it to
	// the model's default.
	Effort string
	// Start, when set, runs before the model is first asked, such as a
	// check that the store session is logged in. An error fails the Run.
	Start func(ctx context.Context, tr Trace) error
	// Finish, when set, runs once the model ends its turn, such as a check
	// that it submitted what the Run is for. An error fails the Run.
	Finish func(ctx context.Context, tr Trace) error
	// LLM, when set, answers this Job instead of the app's model, as in the
	// demo's scripted Runs.
	LLM LLM
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

// StopError is a tool failure the model can't recover from, such as a
// logged-out store session: it ends the Run instead of going back to the
// model.
type StopError struct{ Err error }

func (e *StopError) Error() string { return e.Err.Error() }
func (e *StopError) Unwrap() error { return e.Err }

// Stop makes err end the Run when a tool returns it.
func Stop(err error) error { return &StopError{Err: err} }
