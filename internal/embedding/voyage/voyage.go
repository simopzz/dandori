// Package voyage is the embedding port over Voyage AI, whose models are
// multilingual, so Recipes are embedded in the Content Language as stored.
package voyage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/simopzz/dandori/internal/embedding"
)

// DefaultModel is the Voyage model Recipes are embedded with by default.
const DefaultModel = "voyage-4-large"

const endpoint = "https://api.voyageai.com/v1/embeddings"

// Embedder embeds texts with a Voyage model.
type Embedder struct {
	key      string
	model    string
	endpoint string
	client   *http.Client
}

// New returns an Embedder using model, DefaultModel when empty, with the
// API key key.
func New(key, model string) *Embedder {
	if model == "" {
		model = DefaultModel
	}
	return &Embedder{key: key, model: model, endpoint: endpoint, client: &http.Client{Timeout: 30 * time.Second}}
}

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return e.model }

// A request holds at most maxTexts texts and, as estimated, maxTokens
// tokens: under Voyage's limits for every model, the tightest being 120K
// tokens.
const (
	maxTexts  = 1000
	maxTokens = 100_000
)

// estimateTokens overestimates the tokens Voyage counts in text, at a token
// per 2 bytes where its tokenizer averages about 4.
func estimateTokens(text string) int { return len(text)/2 + 1 }

// Embed implements embedding.Embedder, in as many requests as Voyage's
// limits take.
func (e *Embedder) Embed(ctx context.Context, purpose embedding.Purpose, texts []string) ([][]float32, error) {
	var vectors [][]float32
	for start := 0; start < len(texts); {
		end, tokens := start, 0
		for end < len(texts) && end-start < maxTexts && (end == start || tokens+estimateTokens(texts[end]) <= maxTokens) {
			tokens += estimateTokens(texts[end])
			end++
		}
		batch, err := e.embed(ctx, purpose, texts[start:end])
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
		start = end
	}
	return vectors, nil
}

// embed embeds texts in one request.
func (e *Embedder) embed(ctx context.Context, purpose embedding.Purpose, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"input": texts, "model": e.model, "input_type": string(purpose)})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.key)
	req.Header.Set("Content-Type", "application/json")
	res, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("voyage: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("voyage: read response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		var failure struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(raw, &failure) == nil && failure.Detail != "" {
			return nil, fmt.Errorf("voyage: %s: %s", res.Status, failure.Detail)
		}
		return nil, fmt.Errorf("voyage: %s", res.Status)
	}
	var out struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("voyage: read response: %w", err)
	}
	vectors := make([][]float32, len(texts))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(texts) {
			return nil, fmt.Errorf("voyage: embedding for text %d of %d", d.Index, len(texts))
		}
		vectors[d.Index] = d.Embedding
	}
	for i, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("voyage: no embedding for text %d", i)
		}
	}
	return vectors, nil
}
