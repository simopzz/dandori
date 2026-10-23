// Package demo is a Run kind for trying the agent loop and trace panel by
// hand, before any real kind of Run exists. It is offered only in demo mode
// (MEALOPS_DEMO=1).
//
// The agent answers a pantry question with two toy tools: one looks food up,
// the other always fails. The model is either scripted (no API calls, a
// retry, a failing tool, slow enough to cancel) or the real Claude.
package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/simopzz/mealops/internal/agent"
	"github.com/simopzz/mealops/internal/agent/fakellm"
)

// Kind is the demo's kind of Run.
const Kind = "demo"

// pace is how long the scripted model and the tools take, so there is
// time to watch the trace and cancel the Run.
const pace = 2 * time.Second

// NewJob builds a demo Run's job; the form's model is "fake" or "claude".
func NewJob(form url.Values) (agent.Job, error) {
	job := agent.Job{
		System: "You help one person keep track of their kitchen. Use the tools to answer; " +
			"if a tool fails, say so plainly and answer with what you have.",
		Prompt: "How many rolled oats do I have, and how much does the open bag weigh?",
		Tools:  []agent.Tool{lookUpPantry, weigh},
	}
	switch form.Get("model") {
	case "fake":
		job.LLM = fakellm.New(
			fakellm.Overloaded().Wait(pace),
			fakellm.CallTools("Let me check the pantry.",
				fakellm.Call{Tool: lookUpPantry.Name, Input: map[string]string{"food": "rolled oats"}}).Wait(pace),
			fakellm.CallTools("Now the open bag.",
				fakellm.Call{Tool: weigh.Name, Input: map[string]string{"food": "rolled oats"}}).Wait(pace),
			fakellm.Reply("You have 500 g of rolled oats in the pantry. "+
				"I couldn't weigh the open bag: the kitchen scale is offline.").Wait(pace),
		)
	case "claude":
		// The app's own model answers.
	default:
		return agent.Job{}, fmt.Errorf("a demo Run's model is fake or claude, not %q", form.Get("model"))
	}
	return job, nil
}

var pantry = map[string]string{
	"rolled oats":    "500 g",
	"chicken breast": "2 × 250 g, frozen",
	"eggs":           "6",
}

var lookUpPantry = agent.Tool{
	Name:        "look_up_pantry",
	Description: "How much of a food is in the kitchen. Knows rolled oats, chicken breast and eggs.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"food":{"type":"string"}},"required":["food"]}`),
	Run: func(ctx context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
		var in struct{ Food string }
		if err := json.Unmarshal(input, &in); err != nil {
			return "", err
		}
		if err := sleep(ctx, pace/2); err != nil {
			return "", err
		}
		if amount, ok := pantry[in.Food]; ok {
			return amount + " of " + in.Food, nil
		}
		return "", fmt.Errorf("there is no %s in the kitchen", in.Food)
	},
}

var weigh = agent.Tool{
	Name:        "weigh",
	Description: "Weigh an open pack of food on the kitchen scale.",
	InputSchema: json.RawMessage(`{"type":"object","properties":{"food":{"type":"string"}},"required":["food"]}`),
	Run: func(ctx context.Context, _ agent.Trace, _ json.RawMessage) (string, error) {
		if err := sleep(ctx, pace/2); err != nil {
			return "", err
		}
		return "", errors.New("the kitchen scale is offline")
	},
}

func sleep(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
