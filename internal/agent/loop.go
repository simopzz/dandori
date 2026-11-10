package agent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Loop runs Briefs against the model.
type Loop struct {
	LLM   LLM
	Model string
	// Retries is how many times a failed request to the model is retried when the
	// failure is worth retrying; Backoff is the first wait, doubled on every retry.
	Retries int
	Backoff time.Duration
	// MaxTurns bounds the requests to the model in one Run, so a model that keeps calling
	// tools can't run forever.
	MaxTurns int
}

// Run carries out b, emitting every step to tr, until the model ends its turn without
// calling a tool. It fails with a *Failure, or ctx's error once ctx is cancelled.
func (l Loop) Run(ctx context.Context, b Brief, tr Trace) error {
	tools := map[string]Tool{}
	specs := make([]ToolSpec, 0, len(b.Tools))
	for _, t := range b.Tools {
		tools[t.Name] = t
		specs = append(specs, ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	req := Request{
		Model:    l.Model,
		System:   b.System,
		Messages: []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: b.Prompt}}}},
		Tools:    specs,
		Effort:   b.Effort,
	}

	for turn := 1; ; turn++ {
		if turn > l.MaxTurns {
			return &Failure{Reason: TurnLimit, Detail: strconv.Itoa(l.MaxTurns)}
		}
		asked := TraceEvent{Kind: Asked, Turn: turn}
		if turn == 1 {
			asked.Text = b.Prompt // what the Run was asked, such as the pasted recipe
		}
		tr.Emit(asked)
		resp, err := l.send(ctx, req, tr)
		if err != nil {
			return err
		}
		if resp.Usage != (Usage{}) { // fakes report none
			tr.Emit(TraceEvent{Kind: Tokens, Usage: &resp.Usage})
		}
		req.Messages = append(req.Messages, Message{Role: RoleAssistant, Content: resp.Content})
		for _, bl := range resp.Content {
			if bl.Type == BlockText && bl.Text != "" {
				tr.Emit(TraceEvent{Kind: Said, Text: bl.Text})
			}
		}

		switch resp.StopReason {
		case StopEndTurn, StopSequence:
			return nil
		case StopToolUse:
			results, err := runTools(ctx, tools, resp.Content, tr)
			if err != nil {
				return err
			}
			req.Messages = append(req.Messages, Message{Role: RoleUser, Content: results})
		case StopMaxTokens:
			return outOfTokens(resp.Content)
		case StopRefusal:
			return &Failure{Reason: ModelRefused, Detail: resp.Refusal}
		default:
			return &Failure{Reason: UnexpectedStop, Detail: string(resp.StopReason)}
		}
	}
}

// send asks the model, retrying failures worth retrying with a growing wait.
func (l Loop) send(ctx context.Context, req Request, tr Trace) (Response, error) {
	wait := l.Backoff
	for attempt := 1; ; attempt++ {
		resp, err := l.LLM.Send(ctx, req)
		if err == nil {
			return resp, nil
		}
		if ctx.Err() != nil {
			return Response{}, ctx.Err()
		}
		retryable, ok := errors.AsType[*RetryableError](err)
		if !ok || attempt > l.Retries {
			return Response{}, &Failure{Reason: ModelFailed, Detail: err.Error()}
		}
		pause := max(wait, retryable.After)
		tr.Emit(TraceEvent{Kind: Retrying, Attempt: attempt, Retries: l.Retries, WaitMillis: pause.Milliseconds(), Text: err.Error()})
		select {
		case <-time.After(pause):
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
		wait *= 2
	}
}

// runTools runs the tool calls of the model's turn, in order, and returns their results.
// A failing tool is reported to the model, not fatal.
func runTools(ctx context.Context, tools map[string]Tool, calls []Block, tr Trace) ([]Block, error) {
	var results []Block
	for _, call := range calls {
		if call.Type != BlockToolUse {
			continue
		}
		tr.Emit(TraceEvent{Kind: Called, Tool: call.ToolName, Text: string(call.Input)})
		result := Block{Type: BlockToolResult, ToolUseID: call.ToolUseID}

		tool, ok := tools[call.ToolName]
		var out string
		var err error
		if !ok {
			err = fmt.Errorf("there is no tool named %q", call.ToolName)
		} else {
			out, err = tool.Run(ctx, call.Input)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			tr.Emit(TraceEvent{Kind: Refused, Tool: call.ToolName, Text: err.Error()})
			result.Text, result.IsError = err.Error(), true
		} else {
			tr.Emit(TraceEvent{Kind: Answered, Tool: call.ToolName, Text: out})
			result.Text = out
		}
		results = append(results, result)
	}
	return results, nil
}

// outOfTokens is why a Run fails when the model used its whole token budget: it says so
// when the budget went on thinking, as nothing was then cut off.
func outOfTokens(content []Block) error {
	for _, b := range content {
		if b.Type != BlockOther {
			return &Failure{Reason: OutOfTokens}
		}
	}
	return &Failure{Reason: OutOfTokensInThought}
}
