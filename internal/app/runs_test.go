package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/agent/fakellm"
	"github.com/simopzz/dandori/internal/app"
	"github.com/simopzz/dandori/internal/apptest"
)

// pantryJob is an agent job with one tool, which looks up how much of a
// food is in the pantry.
func pantryJob() agent.Job {
	return agent.Job{
		System: "You check the pantry.",
		Prompt: "How many oats do I have?",
		Tools: []agent.Tool{{
			Name:        "look_up_pantry",
			Description: "How much of a food is in the pantry.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"food":{"type":"string"}},"required":["food"]}`),
			Run: func(_ context.Context, _ agent.Trace, input json.RawMessage) (string, error) {
				var in struct{ Food string }
				if err := json.Unmarshal(input, &in); err != nil {
					return "", err
				}
				return "500 g " + in.Food, nil
			},
		}},
	}
}

// startWith starts the app with the fake LLM and the given jobs.
func startWith(t *testing.T, llm agent.LLM, jobs map[string]agent.Job) *apptest.Harness {
	t.Helper()
	opts := []apptest.Option{apptest.WithPorts(func(p *app.Ports) { p.LLM = llm })}
	for kind, job := range jobs {
		opts = append(opts, apptest.WithJob(kind, job))
	}
	return apptest.Start(t, opts...)
}

// text is what an event or page shows, without markup: of a whole page,
// only its main content, so a failing test doesn't print the shell.
func text(s string) string {
	if _, main, ok := strings.Cut(s, "<main"); ok {
		s, _, _ = strings.Cut(main, "</main>")
		s = "<main" + s
	}
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteRune(' ')
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(html.UnescapeString(b.String())), " ")
}

// readUntil reads events until one shows want, returning everything read.
func readUntil(t *testing.T, s *apptest.Stream, want string) []apptest.Event {
	t.Helper()
	var read []apptest.Event
	for {
		ev, ok := s.Next(t)
		if !ok {
			t.Fatalf("stream ended before showing %q; it showed:\n%s", want, shown(read))
		}
		read = append(read, ev)
		if strings.Contains(text(ev.Data), want) {
			return read
		}
	}
}

// shown is the text of every trace event, one per line.
func shown(events []apptest.Event) string {
	var lines []string
	for _, ev := range events {
		lines = append(lines, ev.Name+": "+text(ev.Data))
	}
	return strings.Join(lines, "\n")
}

func TestRunIsStreamedLiveToTheTracePanel(t *testing.T) {
	thinking := make(chan struct{})
	llm := fakellm.New(
		fakellm.CallTools("Let me look.", fakellm.Call{Tool: "look_up_pantry", Input: map[string]string{"food": "rolled oats"}}).After(thinking),
		fakellm.Reply("You have 500 g of rolled oats."),
	)
	h := startWith(t, llm, map[string]agent.Job{"pantry": pantryJob()})

	run := h.StartRun(t, "pantry")
	page := h.Get(t, run)
	if !strings.Contains(text(page.Body), "Running") {
		t.Errorf("trace panel of a Run in progress does not say it is running:\n%s", page.Body)
	}

	stream := h.Stream(t, run+"/events", "")
	// The model hasn't answered yet, but the Run's start is already shown.
	readUntil(t, stream, "Asking the model")
	close(thinking)

	events := readUntil(t, stream, "You have 500 g of rolled oats.")
	trace := shown(events)
	for _, want := range []string{
		"Let me look.",
		`Tool call look_up_pantry {"food":"rolled oats"}`,
		"Tool result look_up_pantry 500 g rolled oats",
	} {
		if !strings.Contains(trace, want) {
			t.Errorf("live trace does not show %q:\n%s", want, trace)
		}
	}

	rest := stream.Rest(t)
	if len(rest) == 0 || rest[len(rest)-1].Name != "end" || rest[len(rest)-1].Data != "succeeded" {
		t.Errorf("stream did not end by saying the Run succeeded:\n%s", shown(rest))
	}
}

// finish starts a Run of kind and reads its live trace until it ends,
// returning its trace panel's path and the events streamed.
func finish(t *testing.T, h *apptest.Harness, kind string) (string, []apptest.Event) {
	t.Helper()
	run := h.StartRun(t, kind)
	return run, h.Stream(t, run+"/events", "").Rest(t)
}

func TestFinishedRunCanBeReviewedFromItsStoredTrace(t *testing.T) {
	llm := fakellm.New(
		fakellm.CallTool("look_up_pantry", map[string]string{"food": "rolled oats"}),
		fakellm.Reply("You have 500 g of rolled oats."),
	)
	h := startWith(t, llm, map[string]agent.Job{"pantry": pantryJob()})
	run, live := finish(t, h, "pantry")

	page := h.Get(t, run)
	panel := text(page.Body)
	for _, want := range []string{
		"pantry",
		"Succeeded",
		`Tool call look_up_pantry {"food":"rolled oats"}`,
		"Tool result look_up_pantry 500 g rolled oats",
		"Agent You have 500 g of rolled oats.",
		"Finished Run succeeded",
	} {
		if !strings.Contains(panel, want) {
			t.Errorf("stored trace does not show %q:\n%s", want, panel)
		}
	}
	if strings.Contains(panel, "Cancel run") || strings.Contains(page.Body, "data-trace-stream") {
		t.Errorf("a finished Run is still offered for cancelling or followed live:\n%s", page.Body)
	}

	list := text(h.Get(t, "/runs").Body)
	if !strings.Contains(list, "pantry Succeeded") {
		t.Errorf("the Run is not listed with its kind and status:\n%s", list)
	}

	// Following the stream later replays the stored trace exactly.
	replay := h.Stream(t, run+"/events", "").Rest(t)
	if shown(replay) != shown(live) {
		t.Errorf("replayed trace differs from the live one.\nlive:\n%s\nreplay:\n%s", shown(live), shown(replay))
	}
	// A reconnecting browser gets only what it missed.
	resumed := h.Stream(t, run+"/events", live[2].ID).Rest(t)
	if shown(resumed) != shown(live[3:]) {
		t.Errorf("resumed stream after event %s:\n%s\nwant:\n%s", live[2].ID, shown(resumed), shown(live[3:]))
	}
}

func TestRunningRunCanBeCancelled(t *testing.T) {
	llm := fakellm.New(fakellm.Hang(), fakellm.Reply("Too late."))
	h := startWith(t, llm, map[string]agent.Job{"pantry": pantryJob()})
	run := h.StartRun(t, "pantry")
	stream := h.Stream(t, run+"/events", "")
	readUntil(t, stream, "Asking the model")

	if !strings.Contains(text(h.Get(t, run).Body), "Cancel run") {
		t.Fatalf("a running Run is not offered for cancelling")
	}
	res := h.PostForm(t, run+"/cancel", nil)
	if res.StatusCode != http.StatusOK || res.Path != run {
		t.Fatalf("cancel = %d at %s, want the trace panel", res.StatusCode, res.Path)
	}

	rest := stream.Rest(t)
	if !strings.Contains(shown(rest), "Finished Run cancelled") {
		t.Errorf("trace does not show the cancellation:\n%s", shown(rest))
	}
	if end := rest[len(rest)-1]; end.Name != "end" || end.Data != "cancelled" {
		t.Errorf("stream ended with %s %q, want end cancelled", end.Name, end.Data)
	}
	if panel := text(h.Get(t, run).Body); !strings.Contains(panel, "Cancelled") || strings.Contains(panel, "Cancel run") {
		t.Errorf("trace panel does not show a cancelled Run:\n%s", panel)
	}
	if n := len(llm.Requests()); n != 1 {
		t.Errorf("the model was asked %d times; a cancelled Run asks no more", n)
	}
	if again := h.PostForm(t, run+"/cancel", nil); again.StatusCode != http.StatusConflict {
		t.Errorf("cancelling an ended Run = %d, want 409", again.StatusCode)
	}
}

func TestErrorsAndRetriesAppearInTheTrace(t *testing.T) {
	job := pantryJob()
	job.Tools = append(job.Tools, agent.Tool{
		Name:        "weigh",
		Description: "Weigh a food on the kitchen scale.",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Run: func(context.Context, agent.Trace, json.RawMessage) (string, error) {
			return "", errors.New("the scale is offline")
		},
	})
	llm := fakellm.New(
		fakellm.Overloaded(),
		fakellm.CallTool("weigh", map[string]string{"food": "rolled oats"}),
		fakellm.Reply("I couldn't weigh them."),
	)
	h := startWith(t, llm, map[string]agent.Job{"pantry": job})
	run, live := finish(t, h, "pantry")

	for name, trace := range map[string]string{"live": shown(live), "stored": text(h.Get(t, run).Body)} {
		for _, want := range []string{
			"Retry Retry 1 of 3",
			"overloaded (529)",
			"Error Tool weigh failed the scale is offline",
			"Agent I couldn't weigh them.",
			"Finished Run succeeded",
		} {
			if !strings.Contains(trace, want) {
				t.Errorf("%s trace does not show %q:\n%s", name, want, trace)
			}
		}
	}

	// The model is told the tool failed, so it can recover.
	requests := llm.Requests()
	told := requests[len(requests)-1].Messages
	result := told[len(told)-1].Content[0]
	if result.Type != agent.BlockToolResult || !result.IsError || result.Text != "the scale is offline" {
		t.Errorf("the model was not told the tool failed; it was sent %+v", result)
	}
}

func TestFailingRunShowsWhyItFailed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script []fakellm.Step
		want   []string
	}{
		{
			name:   "model unavailable",
			script: []fakellm.Step{fakellm.Fail(errors.New("invalid x-api-key (401)"))},
			want:   []string{"Error Run failed asking the model: invalid x-api-key (401)"},
		},
		{
			name: "retries exhausted",
			script: []fakellm.Step{
				fakellm.Overloaded(), fakellm.Overloaded(), fakellm.Overloaded(), fakellm.Overloaded(),
			},
			want: []string{"Retry 1 of 3", "Retry 3 of 3", "Error Run failed asking the model: overloaded (529)"},
		},
		{
			name:   "answer cut off",
			script: []fakellm.Step{fakellm.Respond(agent.Response{StopReason: agent.StopMaxTokens})},
			want:   []string{"Error Run failed the model's answer was cut off at its token limit"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := startWith(t, fakellm.New(tc.script...), map[string]agent.Job{"pantry": pantryJob()})
			run, live := finish(t, h, "pantry")

			if end := live[len(live)-1]; end.Name != "end" || end.Data != "failed" {
				t.Errorf("stream ended with %s %q, want end failed", end.Name, end.Data)
			}
			panel := text(h.Get(t, run).Body)
			for _, want := range append(tc.want, "Failed", "Finished Run failed") {
				if !strings.Contains(panel, want) {
					t.Errorf("trace does not show %q:\n%s", want, panel)
				}
			}
		})
	}
}

func TestPanickingToolFailsOnlyItsRun(t *testing.T) {
	job := pantryJob()
	job.Tools[0].Run = func(context.Context, agent.Trace, json.RawMessage) (string, error) {
		var pantry map[string]string
		pantry["oats"] = "500 g" //nolint:staticcheck // a bug: the map was never made
		return "", nil
	}
	llm := fakellm.New(
		fakellm.CallTool("look_up_pantry", map[string]string{"food": "rolled oats"}),
		fakellm.Reply("You have 500 g of rolled oats."),
		fakellm.Reply("Still here."),
	)
	h := startWith(t, llm, map[string]agent.Job{"pantry": job, "chat": {Prompt: "Hello?"}})
	run, live := finish(t, h, "pantry")

	if end := live[len(live)-1]; end.Name != "end" || end.Data != "failed" {
		t.Errorf("stream ended with %s %q, want end failed", end.Name, end.Data)
	}
	panel := text(h.Get(t, run).Body)
	for _, want := range []string{"Error Run failed", "assignment to entry in nil map", "Failed"} {
		if !strings.Contains(panel, want) {
			t.Errorf("trace does not show %q:\n%s", want, panel)
		}
	}
	// Dandori is still up and runs the next Run.
	if _, again := finish(t, h, "chat"); !strings.Contains(shown(again), "Finished Run succeeded") {
		t.Errorf("a Run after the panic did not succeed:\n%s", shown(again))
	}
}

func TestRunGivesUpWhenTheModelNeverFinishes(t *testing.T) {
	var script []fakellm.Step
	for range 50 {
		script = append(script, fakellm.CallTool("look_up_pantry", map[string]string{"food": "rolled oats"}))
	}
	h := startWith(t, fakellm.New(script...), map[string]agent.Job{"pantry": pantryJob()})
	run, _ := finish(t, h, "pantry")
	if panel := text(h.Get(t, run).Body); !strings.Contains(panel, "Error Run failed the model did not finish within 50 turns") {
		t.Errorf("trace does not say the model never finished:\n%s", panel)
	}
}

func TestRunInterruptedByACrashIsFailedOnRestart(t *testing.T) {
	h := startWith(t, fakellm.New(fakellm.Hang()), map[string]agent.Job{"pantry": pantryJob()})
	run := h.StartRun(t, "pantry")
	readUntil(t, h.Stream(t, run+"/events", ""), "Asking the model")

	h.Crash(t)
	page := h.Get(t, run)
	panel := text(page.Body)
	for _, want := range []string{"Failed", "Error Run failed Dandori stopped while it was running", "Finished Run failed"} {
		if !strings.Contains(panel, want) {
			t.Errorf("after a restart the interrupted Run does not show %q:\n%s", want, panel)
		}
	}
	if strings.Contains(page.Body, "data-trace-stream") {
		t.Errorf("the interrupted Run is still followed live:\n%s", page.Body)
	}
	if end := h.Stream(t, run+"/events", "").Rest(t); end[len(end)-1].Data != "failed" {
		t.Errorf("stream of the interrupted Run ended with %q, want failed", end[len(end)-1].Data)
	}
}

func TestDemoRunsAreOfferedOnlyInDemoMode(t *testing.T) {
	h := apptest.Start(t)
	if strings.Contains(text(h.Get(t, "/runs").Body), "Start demo run") {
		t.Errorf("demo Runs are offered outside demo mode")
	}
	if res := h.PostForm(t, "/runs", url.Values{"kind": {"demo"}, "model": {"fake"}}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("starting a demo Run outside demo mode = %d, want 400", res.StatusCode)
	}

	// No LLM port: the scripted demo must not need one.
	h = apptest.Start(t, apptest.WithConfig(func(c *app.Config) { c.Demo = true }))
	if list := text(h.Get(t, "/runs").Body); !strings.Contains(list, "Start demo run") {
		t.Fatalf("demo mode does not offer a demo Run:\n%s", list)
	}
	res := h.PostForm(t, "/runs", url.Values{"kind": {"demo"}, "model": {"fake"}})
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/runs/") {
		t.Fatalf("starting a demo Run = %d at %s, want its trace panel", res.StatusCode, res.Path)
	}
	stream := h.Stream(t, res.Path+"/events", "")
	readUntil(t, stream, "demo run with a scripted model")
	readUntil(t, stream, "Retry 1 of 3")
	h.PostForm(t, res.Path+"/cancel", nil)
	if rest := stream.Rest(t); rest[len(rest)-1].Data != "cancelled" {
		t.Errorf("demo Run was not cancelled:\n%s", shown(rest))
	}
	if bad := h.PostForm(t, "/runs", url.Values{"kind": {"demo"}, "model": {"gpt"}}); bad.StatusCode != http.StatusBadRequest {
		t.Errorf("a demo Run with an unknown model = %d, want 400", bad.StatusCode)
	}
}

func TestRunWithoutAnLLMFailsClearly(t *testing.T) {
	h := apptest.Start(t, apptest.WithJob("pantry", pantryJob()))
	run, _ := finish(t, h, "pantry")
	if panel := text(h.Get(t, run).Body); !strings.Contains(panel, "Error Run failed no LLM is configured") {
		t.Errorf("trace does not say there is no LLM:\n%s", panel)
	}
}

func TestUnknownKindOfRunIsRejected(t *testing.T) {
	h := apptest.Start(t)
	res := h.PostForm(t, "/runs", url.Values{"kind": {"teleport"}})
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("starting an unknown kind of Run = %d, want 400", res.StatusCode)
	}
	if list := text(h.Get(t, "/runs").Body); !strings.Contains(list, "No agent has run yet.") {
		t.Errorf("a rejected Run was recorded:\n%s", list)
	}
}
