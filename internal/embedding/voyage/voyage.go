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
	key    string
	model  string
	client *http.Client
}

// New returns an Embedder using model, DefaultModel when empty, with the
// API key key.
func New(key, model string) *Embedder {
	if model == "" {
		model = DefaultModel
	}
	return &Embedder{key: key, model: model, client: &http.Client{Timeout: 30 * time.Second}}
}

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return e.model }

// Embed implements embedding.Embedder.
func (e *Embedder) Embed(ctx context.Context, purpose embedding.Purpose, texts []string) ([][]float32, error) {
	body, err := json.Marshal(map[string]any{"input": texts, "model": e.model, "input_type": string(purpose)})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
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
