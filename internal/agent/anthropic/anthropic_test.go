package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/simopzz/dandori/internal/agent"
)

// sent is one request the fake API received.
type sent struct {
	header http.Header
	body   map[string]any
	raw    []byte
}

// api is a fake Claude API answering every request with status and reply, recording
// what it was sent.
func api(t *testing.T, status int, reply string) (*LLM, *[]sent) {
	t.Helper()
	var got []sent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		s := sent{header: r.Header.Clone(), raw: raw}
		if err := json.Unmarshal(raw, &s.body); err != nil {
			t.Errorf("request body: %v", err)
		}
		got = append(got, s)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(srv.Close)
	return New(option.WithBaseURL(srv.URL), option.WithAPIKey("test")), &got
}

const toolCall = `{"id":"msg_1","type":"message","role":"assistant","model":"m","stop_reason":"tool_use",
	"content":[{"type":"thinking","thinking":"","signature":"sig"},
		{"type":"tool_use","id":"tu_1","name":"submit_recipe","input":{"name":"Pasta"}}],
	"usage":{"input_tokens":1200,"output_tokens":300,"output_tokens_details":{"thinking_tokens":250},
		"cache_read_input_tokens":4000,"cache_creation_input_tokens":900}}`

var parse = agent.Request{
	Model:    "claude-opus-5-5",
	System:   "You read recipes.",
	Messages: []agent.Message{{Role: agent.RoleUser, Content: []agent.Block{{Type: agent.BlockText, Text: "Pasta with tomato"}}}},
	Tools:    []agent.ToolSpec{{Name: "submit_recipe", Description: "Submit it.", InputSchema: json.RawMessage(`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`)}},
	Effort:   "medium",
}

func TestCacheBreakpointsAndAByteStableSystemPrompt(t *testing.T) {
	llm, got := api(t, 200, toolCall)
	first, err := llm.Send(context.Background(), parse)
	if err != nil {
		t.Fatal(err)
	}
	second := parse
	second.Messages = append(second.Messages,
		agent.Message{Role: agent.RoleAssistant, Content: first.Content},
		agent.Message{Role: agent.RoleUser, Content: []agent.Block{{Type: agent.BlockToolResult, ToolUseID: "tu_1", Text: "saved"}}})
	if _, err := llm.Send(context.Background(), second); err != nil {
		t.Fatal(err)
	}

	systems := make([]string, 0, 2)
	for _, s := range *got {
		if cc, _ := s.body["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
			t.Errorf("top-level cache_control = %v, want ephemeral", s.body["cache_control"])
		}
		system, _ := s.body["system"].([]any)
		if len(system) != 1 {
			t.Fatalf("system = %v, want one block", s.body["system"])
		}
		if cc, _ := system[0].(map[string]any)["cache_control"].(map[string]any); cc["type"] != "ephemeral" {
			t.Errorf("system block's cache_control = %v, want ephemeral", cc)
		}
		b, _ := json.Marshal(s.body["system"])
		systems = append(systems, string(b))
	}
	if systems[0] != systems[1] {
		t.Errorf("system prompt changed between turns:\n%s\n%s", systems[0], systems[1])
	}
	// The model's own turn, thinking included, goes back as it came.
	if !strings.Contains(string((*got)[1].raw), `"signature":"sig"`) {
		t.Errorf("the thinking block was not sent back: %s", (*got)[1].raw)
	}
}

func TestFallbackAndEffort(t *testing.T) {
	for _, effort := range []string{"medium", "high"} {
		llm, got := api(t, 200, toolCall)
		req := parse
		req.Effort = effort
		if _, err := llm.Send(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		s := (*got)[0]
		if s.body["fallbacks"] != "default" {
			t.Errorf("fallbacks = %v, want default", s.body["fallbacks"])
		}
		if beta := s.header.Get("anthropic-beta"); !strings.Contains(beta, "server-side-fallback-2026-07-01") {
			t.Errorf("anthropic-beta = %q, want the fallback's beta", beta)
		}
		if cfg, _ := s.body["output_config"].(map[string]any); cfg["effort"] != effort {
			t.Errorf("effort = %v, want %s", cfg["effort"], effort)
		}
	}
}

func TestUsageIncludesTheCache(t *testing.T) {
	llm, _ := api(t, 200, toolCall)
	resp, err := llm.Send(context.Background(), parse)
	if err != nil {
		t.Fatal(err)
	}
	want := agent.Usage{Input: 1200, Output: 300, Thinking: 250, CacheRead: 4000, CacheWrite: 900}
	if resp.Usage != want {
		t.Errorf("usage = %+v, want %+v", resp.Usage, want)
	}
}

func TestARefusalSaysWhy(t *testing.T) {
	llm, _ := api(t, 200, `{"id":"msg_1","type":"message","role":"assistant","model":"m","stop_reason":"refusal",
		"stop_details":{"type":"refusal","category":"bio","explanation":"Not this one."},
		"content":[],"usage":{"input_tokens":1,"output_tokens":1}}`)
	resp, err := llm.Send(context.Background(), parse)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StopReason != agent.StopRefusal || resp.Refusal != "bio: Not this one." {
		t.Errorf("stop = %s, refusal = %q", resp.StopReason, resp.Refusal)
	}
}

func TestFailuresAreClassifiedAndNotRetriedByTheSDK(t *testing.T) {
	for _, tc := range []struct {
		status    int
		retryable bool
	}{
		{429, true}, {529, true}, {500, true}, {408, true},
		{400, false}, {401, false}, {404, false},
	} {
		llm, got := api(t, tc.status, `{"type":"error","error":{"type":"x","message":"no"}}`)
		_, err := llm.Send(context.Background(), parse)
		retry, ok := errors.AsType[*agent.RetryableError](err)
		if ok != tc.retryable {
			t.Errorf("%d: retryable = %v, want %v (%v)", tc.status, ok, tc.retryable, err)
		}
		if ok && retry.After != 7*time.Second {
			t.Errorf("%d: after = %v, want Retry-After's 7s", tc.status, retry.After)
		}
		if len(*got) != 1 {
			t.Errorf("%d: the SDK sent %d requests, want 1", tc.status, len(*got))
		}
	}
}

func TestARequestHasATimeLimit(t *testing.T) {
	var calls atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) }) // runs first, so Close isn't left waiting
	llm := New(option.WithBaseURL(srv.URL), option.WithAPIKey("test"), option.WithRequestTimeout(50*time.Millisecond))

	_, err := llm.Send(context.Background(), parse)
	if _, ok := errors.AsType[*agent.RetryableError](err); !ok {
		t.Errorf("a timed-out request should be worth retrying, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("sent %d requests, want 1", calls.Load())
	}
	if requestTimeout != 5*time.Minute {
		t.Errorf("requestTimeout = %v, want 5m", requestTimeout)
	}
}
