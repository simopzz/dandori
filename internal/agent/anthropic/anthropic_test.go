package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/simopzz/dandori/internal/agent"
)

// send sends req to an LLM whose API answers with reply, and returns the
// body the API was sent and the response.
func send(t *testing.T, req agent.Request, reply string) (map[string]any, agent.Response) {
	t.Helper()
	var sent map[string]any
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &sent); err != nil {
			t.Errorf("request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
	t.Cleanup(api.Close)

	resp, err := New(option.WithBaseURL(api.URL), option.WithAPIKey("test")).Send(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	return sent, resp
}

const thinkingOnly = `{"id":"msg_1","type":"message","role":"assistant","model":"m","stop_reason":"max_tokens",
	"content":[{"type":"thinking","thinking":"","signature":"sig"}],
	"usage":{"input_tokens":1200,"output_tokens":16000,"output_tokens_details":{"thinking_tokens":16000}}}`

func TestSendEffort(t *testing.T) {
	for _, effort := range []string{"", "medium"} {
		sent, _ := send(t, agent.Request{Model: "m", Effort: effort}, thinkingOnly)
		cfg, set := sent["output_config"].(map[string]any)
		switch {
		case effort == "" && set && cfg["effort"] != nil:
			t.Errorf("no effort was asked for, but %v was sent", cfg["effort"])
		case effort != "" && cfg["effort"] != effort:
			t.Errorf("effort %q was sent as %v", effort, cfg["effort"])
		}
	}
}

func TestSendReportsUsage(t *testing.T) {
	_, resp := send(t, agent.Request{Model: "m"}, thinkingOnly)

	want := agent.Usage{InputTokens: 1200, OutputTokens: 16000, ThinkingTokens: 16000}
	if resp.Usage != want {
		t.Errorf("usage = %+v, want %+v", resp.Usage, want)
	}
}
