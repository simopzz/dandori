// Package fakeembed is a deterministic embedder for tests: a bag of words,
// so texts sharing more words are closer. A test can make it fail.
package fakeembed

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"unicode"

	"github.com/simopzz/dandori/internal/embedding"
)

// dimensions is the size of every vector: large enough that the few words
// of a test rarely share one.
const dimensions = 512

// ErrDown is what Embed returns while the embedder is failing.
var ErrDown = errors.New("the embedding provider is down")

// Embedder is a fake embedder. It is safe to change while the app uses it.
type Embedder struct {
	mu   sync.Mutex
	down map[embedding.Purpose]bool
}

// New returns a working fake embedder.
func New() *Embedder { return &Embedder{down: map[embedding.Purpose]bool{}} }

// Fail makes embedding texts for purpose fail, or work again.
func (e *Embedder) Fail(purpose embedding.Purpose, fail bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.down[purpose] = fail
}

// Model implements embedding.Embedder.
func (e *Embedder) Model() string { return "fake-bag-of-words" }

// Embed implements embedding.Embedder: each word adds to one dimension, and
// the vector is normalised.
func (e *Embedder) Embed(_ context.Context, purpose embedding.Purpose, texts []string) ([][]float32, error) {
	e.mu.Lock()
	down := e.down[purpose]
	e.mu.Unlock()
	if down {
		return nil, ErrDown
	}
	vectors := make([][]float32, len(texts))
	for i, text := range texts {
		v := make([]float32, dimensions)
		var norm float64
		for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
			h := fnv.New32a()
			h.Write([]byte(word))
			v[h.Sum32()%dimensions]++
		}
		for _, x := range v {
			norm += float64(x * x)
		}
		if norm == 0 {
			// A text without words is no closer to one thing than another,
			// but a zero vector has no direction at all.
			v[0], norm = 1, 1
		}
		for j := range v {
			v[j] /= float32(math.Sqrt(norm))
		}
		vectors[i] = v
	}
	return vectors, nil
}
