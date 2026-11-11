package runs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// Kind is what a Run's page tells of its kind of Run: its name, the name of what the User
// gives a Run of it, and the page of what it left them, named ResultName, or "" for none.
// Again, if set, is where a failed Run's "Try again" posts what it was asked, as text.
type Kind struct {
	Name, AskedName, ResultName web.Message
	Result                      func(ctx context.Context, runID int64) (string, error)
	Again                       string
}

// Handlers serve the Runs page, a Run's page and its live Trace, and cancel a Run.
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
	if kind.Result != nil && run.Status == Succeeded {
		res.Text = web.T(ctx, kind.ResultName)
		if res.Href, err = kind.Result(ctx, id); err != nil {
			httpx.ServerError(w, r, h.Log, "find what the Run left", err)
			return
		}
	}
	// The page fetches itself as its Run ends: a Run that left something goes on to it.
	if r.URL.Query().Has("ended") && res.Href != "" {
		w.Header().Set("HX-Location", res.Href)
		return
	}
	again := ""
	if run.Status == Failed {
		again = kind.Again
	}
	httpx.Render(w, r, h.Log, http.StatusOK, runPage(h.Clock.Now(), run, web.T(ctx, kind.Name), web.T(ctx, kind.AskedName), res, again))
}

// List is the signed-in User's Runs, newest first.
func (h Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Engine.List(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Runs", err)
		return
	}
	names := map[string]web.Message{}
	for name, k := range h.Kinds {
		names[name] = k.Name
	}
	httpx.Render(w, r, h.Log, http.StatusOK, runsPage(h.Clock.Now(), list, names))
}

// Cancel cancels the signed-in User's Run if it is going, and goes back to its page,
// which shows how it ended.
func (h Handlers) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	found, err := h.Engine.Cancel(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "cancel the Run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, runURL(id), http.StatusSeeOther)
}

// Stream sends the signed-in User's Run's Trace Events as server-sent events: the stored
// ones after the last the browser has (Last-Event-ID, else ?after=), then each as it
// happens until the Run ends, then an "end" event. Trace Events are unnamed messages,
// the only ones htmx's hx-sse swaps in.
func (h Handlers) Stream(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	_, found, err := h.Engine.Status(ctx, id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))
	if last := r.Header.Get("Last-Event-ID"); last != "" {
		after, _ = strconv.Atoi(last)
	}
	// Follow before reading what is stored, so no Trace Event falls between.
	live, stop, following := h.Engine.Follow(id)
	if following {
		defer stop()
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher := http.NewResponseController(w)
	zone := h.Clock.Now().Location()
	send := func(ev StoredTraceEvent) bool {
		if ev.Seq <= after {
			return true
		}
		var b bytes.Buffer
		if err := traceEvent(ev, zone).Render(ctx, &b); err != nil {
			h.Log.ErrorContext(ctx, "render a Trace Event", "err", err)
			return false
		}
		writeSSE(w, strconv.Itoa(ev.Seq), "", b.String())
		after = ev.Seq
		return true
	}
	catchUp := func() bool {
		stored, err := h.Engine.TraceAfter(ctx, id, after)
		if err != nil {
			h.Log.ErrorContext(ctx, "load the Trace", "err", err)
			return false
		}
		for _, ev := range stored {
			if !send(ev) {
				return false
			}
		}
		return flusher.Flush() == nil
	}
	if !catchUp() {
		return
	}
	for following {
		select {
		case ev, open := <-live:
			if !open { // the Run ended, or this fell behind: catch up from what is stored
				if !catchUp() {
					return
				}
				following = false
				continue
			}
			if !send(ev) || flusher.Flush() != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
	status, _, err := h.Engine.Status(ctx, id)
	if err != nil || status == Going {
		// It fell behind, or nothing here is running it: hang up without an end, and the
		// browser reconnects.
		return
	}
	writeSSE(w, "", "end", string(status))
	_ = flusher.Flush()
}

// writeSSE writes one server-sent event; one without a name is a message.
func writeSSE(w io.Writer, id, name, data string) {
	if id != "" {
		fmt.Fprintf(w, "id: %s\n", id)
	}
	if name != "" {
		fmt.Fprintf(w, "event: %s\n", name)
	}
	data = strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(data)
	for line := range strings.SplitSeq(data, "\n") {
		fmt.Fprintf(w, "data: %s\n", line)
	}
	fmt.Fprint(w, "\n")
}
