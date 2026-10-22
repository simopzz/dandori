package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Loop runs Jobs against the model.
type Loop struct {
	LLM   LLM
	Model string
	// Retries is how many times a failed request to the model is retried
	// when the failure is worth retrying; Backoff is the first wait, doubled
	// on every retry.
	Retries int
	Backoff time.Duration
	// MaxTurns bounds the requests to the model in one Run, so a model that
	// keeps calling tools can't run forever.
	MaxTurns int
}

// Run carries out job, emitting every step to tr, until the model ends its
// turn without calling a tool. It returns ctx's error once ctx is cancelled.
func (l Loop) Run(ctx context.Context, job Job, tr Trace) error {
	tools := map[string]Tool{}
	specs := make([]ToolSpec, 0, len(job.Tools))
	for _, t := range job.Tools {
		tools[t.Name] = t
		specs = append(specs, ToolSpec{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	req := Request{
		Model:    l.Model,
		System:   job.System,
		Messages: []Message{{Role: RoleUser, Content: []Block{{Type: BlockText, Text: job.Prompt}}}},
		Tools:    specs,
	}

	for turn := 1; ; turn++ {
		if turn > l.MaxTurns {
			return fmt.Errorf("the model did not finish within %d turns", l.MaxTurns)
		}
		tr.Emit(Event{Kind: EventStep, Summary: fmt.Sprintf("Asking the model (turn %d)", turn)})
		resp, err := l.send(ctx, req, tr)
		if err != nil {
			return err
		}
		req.Messages = append(req.Messages, Message{Role: RoleAssistant, Content: resp.Content})
		for _, b := range resp.Content {
			if b.Type == BlockText && b.Text != "" {
				tr.Emit(Event{Kind: EventMessage, Summary: b.Text})
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
			return errors.New("the model's answer was cut off at its token limit")
		case StopRefusal:
			return errors.New("the model declined to continue")
		default:
			return fmt.Errorf("the model stopped for an unexpected reason: %q", resp.StopReason)
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
		var retryable *RetryableError
		if !errors.As(err, &retryable) || attempt > l.Retries {
			return Response{}, fmt.Errorf("asking the model: %w", err)
		}
		pause := max(wait, retryable.After)
		tr.Emit(Event{
			Kind:    EventRetry,
			Summary: fmt.Sprintf("Retry %d of %d in %s", attempt, l.Retries, pause),
			Detail:  err.Error(),
		})
		select {
		case <-time.After(pause):
		case <-ctx.Done():
			return Response{}, ctx.Err()
		}
		wait *= 2
	}
}

// runTools runs the tool calls of the model's turn, in order, and returns
// their results. A failing tool is reported to the model, not fatal.
func runTools(ctx context.Context, tools map[string]Tool, calls []Block, tr Trace) ([]Block, error) {
	var results []Block
	for _, call := range calls {
		if call.Type != BlockToolUse {
			continue
		}
		tr.Emit(Event{Kind: EventToolCall, Summary: call.ToolName, Detail: string(call.Input)})
		result := Block{Type: BlockToolResult, ToolUseID: call.ToolUseID}

		tool, ok := tools[call.ToolName]
		var out string
		var err error
		if !ok {
			err = fmt.Errorf("there is no tool named %q", call.ToolName)
		} else {
			out, err = tool.Run(ctx, tr, call.Input)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if err != nil {
			tr.Emit(Event{Kind: EventError, Summary: "Tool " + call.ToolName + " failed", Detail: err.Error()})
			result.Text, result.IsError = err.Error(), true
		} else {
			tr.Emit(Event{Kind: EventToolResult, Summary: call.ToolName, Detail: out})
			result.Text = out
		}
		results = append(results, result)
	}
	return results, nil
}
