// Package anthropic is the LLM port over the Claude API.
package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/simopzz/mealops/internal/agent"
)

// maxTokens bounds one answer of the model; it keeps a non-streaming
// request well inside the SDK's HTTP timeout.
const maxTokens = 16000

// LLM sends requests to Claude.
type LLM struct {
	client sdk.Client
}

// New returns the Claude LLM. Credentials come from the environment
// (ANTHROPIC_API_KEY, or a profile from `ant auth login`). The SDK does not
// retry: the agent loop does, so retries show in the trace.
func New(opts ...option.RequestOption) *LLM {
	opts = append([]option.RequestOption{option.WithMaxRetries(0)}, opts...)
	return &LLM{client: sdk.NewClient(opts...)}
}

// Send implements agent.LLM.
func (l *LLM) Send(ctx context.Context, req agent.Request) (agent.Response, error) {
	params := sdk.MessageNewParams{
		Model:     sdk.Model(req.Model),
		MaxTokens: maxTokens,
	}
	if req.System != "" {
		params.System = []sdk.TextBlockParam{{Text: req.System}}
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

	msg, err := l.client.Messages.New(ctx, params)
	if err != nil {
		return agent.Response{}, classify(ctx, err)
	}
	resp := agent.Response{StopReason: agent.StopReason(msg.StopReason)}
	for _, b := range msg.Content {
		block := agent.Block{Type: agent.BlockOther, Raw: json.RawMessage(b.RawJSON())}
		switch v := b.AsAny().(type) {
		case sdk.TextBlock:
			block.Type, block.Text = agent.BlockText, v.Text
		case sdk.ToolUseBlock:
			block.Type, block.ToolUseID, block.ToolName, block.Input = agent.BlockToolUse, v.ID, v.Name, v.Input
		}
		resp.Content = append(resp.Content, block)
	}
	return resp, nil
}

func toolParam(t agent.ToolSpec) (sdk.ToolUnionParam, error) {
	var schema map[string]any
	if err := json.Unmarshal(t.InputSchema, &schema); err != nil {
		return sdk.ToolUnionParam{}, fmt.Errorf("input schema of tool %s: %w", t.Name, err)
	}
	input := sdk.ToolInputSchemaParam{Properties: schema["properties"], ExtraFields: map[string]any{}}
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
	return sdk.ToolUnionParam{OfTool: &sdk.ToolParam{
		Name:        t.Name,
		Description: sdk.String(t.Description),
		InputSchema: input,
	}}, nil
}

func messageParam(m agent.Message) (sdk.MessageParam, error) {
	var blocks []sdk.ContentBlockParamUnion
	for _, b := range m.Content {
		block, err := blockParam(b)
		if err != nil {
			return sdk.MessageParam{}, err
		}
		blocks = append(blocks, block)
	}
	if m.Role == agent.RoleAssistant {
		return sdk.NewAssistantMessage(blocks...), nil
	}
	return sdk.NewUserMessage(blocks...), nil
}

func blockParam(b agent.Block) (sdk.ContentBlockParamUnion, error) {
	if len(b.Raw) > 0 {
		// The model's own block, thinking included, goes back unchanged.
		var u sdk.ContentBlockUnion
		if err := json.Unmarshal(b.Raw, &u); err != nil {
			return sdk.ContentBlockParamUnion{}, fmt.Errorf("replay content block: %w", err)
		}
		return u.ToParam(), nil
	}
	switch b.Type {
	case agent.BlockText:
		return sdk.NewTextBlock(b.Text), nil
	case agent.BlockToolUse:
		return sdk.NewToolUseBlock(b.ToolUseID, b.Input, b.ToolName), nil
	case agent.BlockToolResult:
		return sdk.NewToolResultBlock(b.ToolUseID, b.Text, b.IsError), nil
	default:
		return sdk.ContentBlockParamUnion{}, fmt.Errorf("cannot send a %s block without its raw form", b.Type)
	}
}

// classify marks the failures worth retrying: rate limits, timeouts,
// overloaded or failing servers, and the network.
func classify(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return err
	}
	var apiErr *sdk.Error
	if !errors.As(err, &apiErr) {
		// Reaching the API failed: the network, or a timeout. Anything else
		// (a request that can't be built) fails the same way every time.
		var netErr net.Error
		if errors.As(err, &netErr) || errors.Is(err, io.ErrUnexpectedEOF) {
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

func retryAfter(res *http.Response) time.Duration {
	if res == nil {
		return 0
	}
	h := res.Header.Get("Retry-After")
	if secs, err := strconv.Atoi(h); err == nil {
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(h); err == nil {
		return max(time.Until(at), 0)
	}
	return 0
}
