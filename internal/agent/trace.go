package agent

import "fmt"

// TraceEventKind is what a Trace Event records. Each is shown by a fixed line, a Message,
// and what the model or a tool said is kept verbatim in its Text.
type TraceEventKind string

const (
	// Started: the Run began, with Model.
	Started TraceEventKind = "started"
	// Asked: the model is asked for its Turn, and on the first Text is what the Run
	// was asked, such as the pasted recipe.
	Asked TraceEventKind = "asked"
	// Said: the model said Text.
	Said TraceEventKind = "said"
	// Called: the model called Tool with the input Text.
	Called TraceEventKind = "called"
	// Answered: Tool answered Text.
	Answered TraceEventKind = "answered"
	// Refused: Tool refused the call, saying Text, which went back to the model.
	Refused TraceEventKind = "refused"
	// Retrying: asking the model failed with Text, and Attempt of Retries asks again
	// after WaitMillis.
	Retrying TraceEventKind = "retrying"
	// Tokens: one model turn used Usage.
	Tokens TraceEventKind = "tokens"
	// Failed: the Run failed for Reason, Text saying more when there is more.
	Failed TraceEventKind = "failed"
	// Ended: the Run ended with Status.
	Ended TraceEventKind = "ended"
)

// TraceEvent is one Trace Event of a Run: its kind and data.
type TraceEvent struct {
	Kind       TraceEventKind `json:"-"`
	Model      string         `json:"model,omitempty"`
	Turn       int            `json:"turn,omitempty"`
	Tool       string         `json:"tool,omitempty"`
	Text       string         `json:"text,omitempty"`
	Attempt    int            `json:"attempt,omitempty"`
	Retries    int            `json:"retries,omitempty"`
	WaitMillis int64          `json:"wait_millis,omitempty"`
	Usage      *Usage         `json:"usage,omitempty"`
	Reason     Reason         `json:"reason,omitempty"`
	Status     string         `json:"status,omitempty"`
}

// Trace receives a Run's Trace Events as they happen. Emit must work after the Run's
// context is cancelled, so a cancellation can still be recorded.
type Trace interface {
	Emit(TraceEvent)
}

// Reason is why a Run failed, shown as a Message.
type Reason string

const (
	TurnLimit            Reason = "turn_limit"
	ModelRefused         Reason = "model_refused"
	OutOfTokens          Reason = "out_of_tokens"
	OutOfTokensInThought Reason = "out_of_tokens_thinking"
	UnexpectedStop       Reason = "unexpected_stop"
	ModelFailed          Reason = "model_failed"
	// NoResult: the model ended its turn without leaving what the Run is for.
	NoResult Reason = "no_result"
	// Crashed, NoModel and Interrupted are the Run engine's own.
	Crashed     Reason = "crashed"
	NoModel     Reason = "no_model"
	Interrupted Reason = "interrupted"
)

// Failure is a Run failing for Reason, Detail saying more, verbatim, when there is more.
type Failure struct {
	Reason Reason
	Detail string
}

func (f *Failure) Error() string { return fmt.Sprintf("%s: %s", f.Reason, f.Detail) }
