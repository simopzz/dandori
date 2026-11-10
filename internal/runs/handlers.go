package runs

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Kind is what a Run's page tells of its kind of Run: its name, and the page of what a
// Run of it left the signed-in User, named ResultName, or "" for nothing.
type Kind struct {
	Name, ResultName web.Message
	Result           func(ctx context.Context, runID int64) (string, error)
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
		res.Text = web.T(ctx, kind.ResultName)
		if res.Href, err = kind.Result(ctx, id); err != nil {
			httpx.ServerError(w, r, h.Log, "find what the Run left", err)
			return
		}
	}
	httpx.Render(w, r, h.Log, http.StatusOK, runPage(h.Clock.Now(), run, web.T(ctx, kind.Name), res))
}
