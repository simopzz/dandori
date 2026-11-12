package runs

import (
	"context"

	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/telemetry"
)

// spanned makes each call to the model a client span under the Run's, as OTel's
// generative-AI conventions describe one: the model, the tokens and why the model
// stopped, never what it was asked or said. The LLM adapter adds what only it knows.
type spanned struct {
	llm    agent.LLM
	tracer oteltrace.Tracer
}

func (s spanned) Send(ctx context.Context, req agent.Request) (agent.Response, error) {
	ctx, span := s.tracer.Start(ctx, "chat "+req.Model, oteltrace.WithSpanKind(oteltrace.SpanKindClient), oteltrace.WithAttributes(
		semconv.GenAIOperationNameChat, semconv.GenAIRequestModel(req.Model), telemetry.RunID.Int64(IDFrom(ctx))))
	defer span.End()
	resp, err := s.llm.Send(ctx, req)
	if err != nil {
		if ctx.Err() == nil { // a call the User or a shutdown cut off didn't fail
			span.SetStatus(codes.Error, err.Error())
		}
		return resp, err
	}
	span.SetAttributes(semconv.GenAIResponseFinishReasons(string(resp.StopReason)))
	if u := resp.Usage; u != (agent.Usage{}) { // fakes report none
		span.SetAttributes(
			// The conventions count the cache's tokens as input; the API doesn't.
			semconv.GenAIUsageInputTokens(int(u.Input+u.CacheRead+u.CacheWrite)),
			semconv.GenAIUsageCacheReadInputTokens(int(u.CacheRead)),
			semconv.GenAIUsageCacheCreationInputTokens(int(u.CacheWrite)),
			semconv.GenAIUsageOutputTokens(int(u.Output)),
			semconv.GenAIUsageReasoningOutputTokens(int(u.Thinking)))
	}
	return resp, nil
}
