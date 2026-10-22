// Package agent is the hand-rolled tool-use loop (ADR-0003): it talks to the model
// through the LLM port, runs the job's tools, and emits every step to a Trace.
package agent

import (
	"context"
	"encoding/json"
	"time"
)

// LLM is the port to the language model: send the conversation and the
// tools, get the model's next turn.
type LLM interface {
	Send(ctx context.Context, req Request) (Response, error)
}

// Request is one call to the model.
type Request struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolSpec
}

// Response is the model's turn.
type Response struct {
	StopReason StopReason
	Content    []Block
}

// StopReason says why the model ended its turn.
type StopReason string

const (
	StopEndTurn   StopReason = "end_turn"
	StopToolUse   StopReason = "tool_use"
	StopMaxTokens StopReason = "max_tokens"
	StopSequence  StopReason = "stop_sequence"
	StopRefusal   StopReason = "refusal"
)

// Role is who said a Message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one turn of the conversation.
type Message struct {
	Role    Role
	Content []Block
}

// BlockType is the kind of a content Block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolUse    BlockType = "tool_use"
	BlockToolResult BlockType = "tool_result"
	// BlockOther is a block the loop does not read, such as thinking; it is
	// sent back to the model unchanged through Raw.
	BlockOther BlockType = "other"
)

// Block is one piece of a Message.
type Block struct {
	Type BlockType
	Text string // text, or a tool result's content

	ToolUseID string // tool_use id, or the tool_use a tool_result answers
	ToolName  string
	Input     json.RawMessage
	IsError   bool

	// Raw is the block as the LLM adapter received it, so the adapter can
	// send the model's own turn back unchanged. Fakes leave it empty.
	Raw json.RawMessage
}

// ToolSpec describes a tool to the model.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// RetryableError marks an LLM failure worth retrying, such as a rate limit
// or an overloaded server. After is how long the server asked us to wait.
type RetryableError struct {
	Err   error
	After time.Duration
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }
