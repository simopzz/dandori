// Package embedding is the embedding port: it turns text into vectors whose
// closeness is closeness in meaning, so the Recipe Library can be searched
// in natural language. The Voyage adapter and the fake for tests implement
// it.
package embedding

import "context"

// Purpose is what a text is embedded for. Some models embed a search query
// differently from the documents it is matched against.
type Purpose string

const (
	Document Purpose = "document"
	Query    Purpose = "query"
)

// Embedder is the port to an embedding model.
type Embedder interface {
	// Model names the model, so vectors from different models, which can't
	// be compared, are never mixed.
	Model() string
	// Embed returns one vector per text, in order.
	Embed(ctx context.Context, purpose Purpose, texts []string) ([][]float32, error)
}
