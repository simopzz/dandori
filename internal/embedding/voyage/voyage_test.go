package voyage

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/embedding"
)

// fakeVoyage serves the embeddings endpoint the way Voyage does for
// voyage-4-large: it rejects a request of more than 1,000 texts or 120K
// tokens, counting a token per 4 bytes, and lists the embeddings in any
// order, each with its index. A text's embedding is its leading number.
func fakeVoyage(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tokens := 0
		for _, text := range req.Input {
			tokens += len(text) / 4
		}
		if len(req.Input) > 1000 || tokens > 120_000 {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, `{"detail": "%d texts, %d tokens: over the limit"}`, len(req.Input), tokens)
			return
		}
		type datum struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}
		var data []datum
		for i, text := range req.Input {
			n, _ := strconv.Atoi(strings.Fields(text)[0])
			data = append(data, datum{Embedding: []float32{float32(n)}, Index: i})
		}
		slices.Reverse(data)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEmbedsMoreThanOneRequestHolds(t *testing.T) {
	tests := []struct {
		name  string
		texts []string
	}{
		{"more texts than a request holds", numbered(2500, "")},
		{"more tokens than a request holds", numbered(6, strings.Repeat("ceci croccanti ", 7000))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := New("key", "")
			e.endpoint = fakeVoyage(t).URL

			vectors, err := e.Embed(context.Background(), embedding.Document, tt.texts)
			if err != nil {
				t.Fatal(err)
			}
			if len(vectors) != len(tt.texts) {
				t.Fatalf("got %d vectors for %d texts", len(vectors), len(tt.texts))
			}
			for i, v := range vectors {
				if v[0] != float32(i) {
					t.Fatalf("vector %d is text %v's", i, v[0])
				}
			}
		})
	}
}

// numbered returns n texts, each its number followed by body.
func numbered(n int, body string) []string {
	texts := make([]string, n)
	for i := range texts {
		texts[i] = strconv.Itoa(i) + " " + body
	}
	return texts
}
