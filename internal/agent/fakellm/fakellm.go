// Package fakellm is a scripted LLM, for tests and the demo: each request to
// the model is answered by the next scripted Step, so agent flows are
// deterministic.
package fakellm

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/simopzz/dandori/internal/agent"
)

// LLM answers requests from its script, in order. It is safe to script more
// steps while a Run is using it.
type LLM struct {
	mu       sync.Mutex
	steps    []Step
	requests []agent.Request
	toolUses int
}

// New returns a fake LLM that answers with steps.
func New(steps ...Step) *LLM { return &LLM{steps: steps} }

// Script adds steps to the end of the script.
func (f *LLM) Script(steps ...Step) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps = append(f.steps, steps...)
}

// Requests returns every request the model was sent, in order.
func (f *LLM) Requests() []agent.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.requests)
}

// Step is one scripted answer of the model.
type Step struct {
	turn  func(f *LLM) agent.Response
	err   error
	hang  bool
	after <-chan struct{}
	wait  time.Duration
}

// Reply ends the model's turn with text.
func Reply(text string) Step {
	return Step{turn: func(*LLM) agent.Response {
		return agent.Response{
			StopReason: agent.StopEndTurn,
			Content:    []agent.Block{{Type: agent.BlockText, Text: text}},
		}
	}}
}

// Call is one tool call the model makes.
type Call struct {
	Tool  string
	Input any
}

// CallTools has the model call tools, saying text first if it isn't empty.
func CallTools(text string, calls ...Call) Step {
	return Step{turn: func(f *LLM) agent.Response {
		var content []agent.Block
		if text != "" {
			content = append(content, agent.Block{Type: agent.BlockText, Text: text})
		}
		for _, c := range calls {
			input, err := json.Marshal(c.Input)
			if err != nil {
				panic(fmt.Sprintf("fakellm: tool input for %s: %v", c.Tool, err))
			}
			f.toolUses++
			content = append(content, agent.Block{
				Type:      agent.BlockToolUse,
				ToolUseID: fmt.Sprintf("toolu_%d", f.toolUses),
				ToolName:  c.Tool,
				Input:     input,
			})
		}
		return agent.Response{StopReason: agent.StopToolUse, Content: content}
	}}
}

// CallTool has the model call one tool.
func CallTool(tool string, input any) Step { return CallTools("", Call{tool, input}) }

// Respond answers with resp as given, e.g. to script an unusual stop reason.
func Respond(resp agent.Response) Step {
	return Step{turn: func(*LLM) agent.Response { return resp }}
}

// Fail makes the request fail with err.
func Fail(err error) Step { return Step{err: err} }

// Overloaded makes the request fail in a way worth retrying.
func Overloaded() Step {
	return Fail(&agent.RetryableError{Err: fmt.Errorf("overloaded (529)")})
}

// Hang never answers: the request blocks until its context is cancelled.
func Hang() Step { return Step{hang: true} }

// After holds the answer back until ch is closed, so a test can act while
// the model is "thinking".
func (s Step) After(ch <-chan struct{}) Step {
	s.after = ch
	return s
}

// Wait has the model take d to answer, as a real one does.
func (s Step) Wait(d time.Duration) Step {
	s.wait = d
	return s
}

// Send implements agent.LLM.
func (f *LLM) Send(ctx context.Context, req agent.Request) (agent.Response, error) {
	f.mu.Lock()
	req.Messages = slices.Clone(req.Messages)
	f.requests = append(f.requests, req)
	n := len(f.requests)
	if len(f.steps) == 0 {
		f.mu.Unlock()
		return agent.Response{}, fmt.Errorf("fakellm: nothing scripted for request %d", n)
	}
	step := f.steps[0]
	f.steps = f.steps[1:]
	f.mu.Unlock()

	if step.hang {
		<-ctx.Done()
		return agent.Response{}, ctx.Err()
	}
	if step.after != nil {
		select {
		case <-step.after:
		case <-ctx.Done():
			return agent.Response{}, ctx.Err()
		}
	}
	if step.wait > 0 {
		select {
		case <-time.After(step.wait):
		case <-ctx.Done():
			return agent.Response{}, ctx.Err()
		}
	}
	if step.err != nil {
		return agent.Response{}, step.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return step.turn(f), nil
}
