package runs

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Kind is what a Run's page tells of its kind of Run.
type Kind struct {
	Name web.Message
	// Result is the link to what the Run id left the signed-in User, text and where it
	// goes, or "" for nothing; nil when no Run of the kind leaves anything.
	Result func(ctx context.Context, runID int64) (text, href string, err error)
}

// Handlers serve a Run's page.
type Handlers struct {
	Engine *Engine
	// Kinds are the kinds of Run, by the name they are started with.
	Kinds map[string]Kind
	Clock clock.Clock
	Log   *slog.Logger
}

// Show is the signed-in User's Run and its Trace so far.
func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	run, found, err := h.Engine.Find(ctx, id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	kind := h.Kinds[run.Kind]
	var res result
	if kind.Result != nil && run.Status == "succeeded" {
		if res.Text, res.Href, err = kind.Result(ctx, id); err != nil {
			httpx.ServerError(w, r, h.Log, "find what the Run left", err)
			return
		}
	}
	httpx.Render(w, r, h.Log, http.StatusOK, runPage(h.Clock.Now(), run, web.T(ctx, kind.Name), res))
}
