// Package anthropic is the LLM port over the Claude API.
package anthropic

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/simopzz/dandori/internal/agent"
)

// maxTokens bounds one answer of the model; it keeps a non-streaming request well
// inside requestTimeout.
const maxTokens = 16000

// requestTimeout is how long one request to the model may take.
const requestTimeout = 5 * time.Minute

// LLM sends requests to Claude.
type LLM struct {
	client sdk.Client
}

// New returns the Claude LLM. Credentials come from the environment (ANTHROPIC_API_KEY,
// or a profile from `ant auth login`). The SDK does not retry: the agent loop does, so
// retries show in the Trace.
func New(opts ...option.RequestOption) *LLM {
	opts = append([]option.RequestOption{option.WithMaxRetries(0), option.WithRequestTimeout(requestTimeout)}, opts...)
	return &LLM{client: sdk.NewClient(opts...)}
}

// Send implements agent.LLM. It goes through the Beta Messages endpoint for the
// server-side refusal fallback.
func (l *LLM) Send(ctx context.Context, req agent.Request) (agent.Response, error) {
	params := sdk.BetaMessageNewParams{
		Model:     req.Model,
		MaxTokens: maxTokens,
		Betas:     []sdk.AnthropicBeta{sdk.AnthropicBetaServerSideFallback2026_07_01},
		Fallbacks: sdk.BetaFallbacksParamOfDefault(),
		// The automatic breakpoint follows the conversation; the system prompt's own
		// keeps it cached for every Run of its kind.
		CacheControl: sdk.NewBetaCacheControlEphemeralParam(),
		OutputConfig: sdk.BetaOutputConfigParam{Effort: sdk.BetaOutputConfigEffort(req.Effort)},
	}
	if req.System != "" {
		params.System = []sdk.BetaTextBlockParam{{Text: req.System, CacheControl: sdk.NewBetaCacheControlEphemeralParam()}}
	}
	for _, t := range req.Tools {
		tool, err := toolParam(t)
		if err != nil {
			return agent.Response{}, err
		}
		params.Tools = append(params.Tools, tool)
	}
	for _, m := range req.Messages {
		msg, err := messageParam(m)
		if err != nil {
			return agent.Response{}, err
		}
		params.Messages = append(params.Messages, msg)
	}

	msg, err := l.client.Beta.Messages.New(ctx, params)
	if err != nil {
		return agent.Response{}, classify(ctx, err)
	}
	resp := agent.Response{
		StopReason: agent.StopReason(msg.StopReason),
		Usage: agent.Usage{
			Input:      msg.Usage.InputTokens,
			Output:     msg.Usage.OutputTokens,
			Thinking:   msg.Usage.OutputTokensDetails.ThinkingTokens,
			CacheRead:  msg.Usage.CacheReadInputTokens,
			CacheWrite: msg.Usage.CacheCreationInputTokens,
		},
	}
	if d := msg.StopDetails; d.Category != "" && d.Explanation != "" {
		resp.Refusal = fmt.Sprintf("%s: %s", d.Category, d.Explanation)
	} else {
		resp.Refusal = cmp.Or(string(d.Category), d.Explanation)
	}
	// A model that declined mid-answer handed over at a fallback block: of what it
	// wrote before, only text goes back, so its thinking isn't replayed (the API
	// rejects it) and its tool calls aren't run.
	last := -1
	for i, b := range msg.Content {
		if b.Type == "fallback" {
			last = i
		}
	}
	for i, b := range msg.Content {
		if i < last && b.Type != "text" {
			continue
		}
		block := agent.Block{Type: agent.BlockOther, Raw: json.RawMessage(b.RawJSON())}
		switch v := b.AsAny().(type) {
		case sdk.BetaTextBlock:
			block.Type, block.Text = agent.BlockText, v.Text
		case sdk.BetaToolUseBlock:
			block.Type, block.ToolUseID, block.ToolName, block.Input = agent.BlockToolUse, v.ID, v.Name, json.RawMessage(v.JSON.Input.Raw())
		}
		resp.Content = append(resp.Content, block)
	}
	return resp, nil
}

func toolParam(t agent.ToolSpec) (sdk.BetaToolUnionParam, error) {
	var schema map[string]any
	if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
		return sdk.BetaToolUnionParam{}, fmt.Errorf("input schema of tool %s: %w", t.Name, err)
	}
	input := sdk.BetaToolInputSchemaParam{Properties: schema["properties"], ExtraFields: map[string]any{}}
	if required, ok := schema["required"].([]any); ok {
		for _, r := range required {
			if name, ok := r.(string); ok {
				input.Required = append(input.Required, name)
			}
		}
	}
	for k, v := range schema {
		if k != "type" && k != "properties" && k != "required" {
			input.ExtraFields[k] = v
		}
	}
	return sdk.BetaToolUnionParam{OfTool: &sdk.BetaToolParam{
		Name:        t.Name,
		Description: sdk.String(t.Description),
		InputSchema: input,
	}}, nil
}

func messageParam(m agent.Message) (sdk.BetaMessageParam, error) {
	var blocks []sdk.BetaContentBlockParamUnion
	for _, b := range m.Content {
		block, err := blockParam(b)
		if err != nil {
			return sdk.BetaMessageParam{}, err
		}
		blocks = append(blocks, block)
	}
	if m.Role == agent.RoleAssistant {
		return sdk.BetaMessageParam{OfAssistant: &sdk.BetaAssistantMessageParam{Content: blocks}}, nil
	}
	return sdk.NewBetaUserMessage(blocks...), nil
}

func blockParam(b agent.Block) (sdk.BetaContentBlockParamUnion, error) {
	if len(b.Raw) > 0 {
		// The model's own block, thinking included, goes back unchanged.
		var u sdk.BetaContentBlockUnion
		if err := json.Unmarshal(b.Raw, &u); err != nil {
			return sdk.BetaContentBlockParamUnion{}, fmt.Errorf("replay content block: %w", err)
		}
		return u.ToParam(), nil
	}
	switch b.Type {
	case agent.BlockText:
		return sdk.NewBetaTextBlock(b.Text), nil
	case agent.BlockToolUse:
		return sdk.NewBetaToolUseBlock(b.ToolUseID, b.Input, b.ToolName), nil
	case agent.BlockToolResult:
		return sdk.NewBetaToolResultBlock(b.ToolUseID, b.Text, b.IsError), nil
	default:
		return sdk.BetaContentBlockParamUnion{}, fmt.Errorf("cannot send a %s block without its raw form", b.Type)
	}
}

// classify marks the failures worth retrying: rate limits, timeouts, overloaded or
// failing servers, and the network.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return err
	}
	var apiErr *sdk.Error
	if !errors.As(err, &apiErr) {
		// Reaching the API failed: the network, or a timeout. Anything else (a request
		// that can't be built) fails the same way every time.
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, context.DeadlineExceeded) {
			return &agent.RetryableError{Err: err}
		}
		return err
	}
	switch code := apiErr.StatusCode; {
	case code == http.StatusRequestTimeout, code == http.StatusConflict,
		code == http.StatusTooManyRequests, code >= 500:
		return &agent.RetryableError{Err: err, After: retryAfter(apiErr.Response)}
	}
	return err
}

// retryAfter is the wait the API asked for, which it gives in seconds.
func retryAfter(res *http.Response) time.Duration {
	if res == nil {
		return 0
	}
	if secs, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil {
		return time.Duration(secs) * time.Second
	}
	return 0
}
