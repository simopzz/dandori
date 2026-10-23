package agent

// EventKind is what a Trace Event records.
type EventKind string

const (
	EventStarted    EventKind = "started"
	EventStep       EventKind = "step"
	EventMessage    EventKind = "message"
	EventToolCall   EventKind = "tool_call"
	EventToolResult EventKind = "tool_result"
	EventRetry      EventKind = "retry"
	EventWait       EventKind = "wait"
	EventError      EventKind = "error"
	EventFinished   EventKind = "finished"
)

// Event is one Trace Event of a Run.
type Event struct {
	Kind    EventKind
	Summary string
	Detail  string
}

// Trace receives a Run's Trace Events as they happen. Emit must work after
// the Run's context is cancelled, so a cancellation can still be recorded.
type Trace interface {
	Emit(Event)
}
