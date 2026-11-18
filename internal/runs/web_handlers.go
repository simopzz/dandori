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

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/clock"
	"github.com/simopzz/dandori/internal/httpx"
	"github.com/simopzz/dandori/internal/web"
)

// KindPage is what the Runs pages tell of one kind of Run. Each feature that starts a
// kind of Run builds its own.
type KindPage struct {
	// Kind is the kind of Run this tells of, as the feature starts it.
	Kind Kind
	// Name names a Run of the kind on the Runs page and as its page's title, such as
	// "Reading a Recipe".
	Name web.Message
	// AskedLabel labels the fold on a Run's page holding what the Run was asked, such as
	// "Pasted recipe" over the recipe text. Without one, the page shows no such fold.
	AskedLabel web.Message
	// ResultLabel is the text of the link from a succeeded Run's page to what it left,
	// such as "Recipe Draft".
	ResultLabel web.Message
	// ResultURL is where what the Run runID left is, or "" if it left nothing: the
	// ResultLabel link goes there, and so does the page of a Run that has just
	// succeeded. Without one, a Run's page links to nothing.
	ResultURL func(ctx context.Context, runID int64) (string, error)
	// TryAgainURL is where a failed Run's "Try again" button posts what the Run was
	// asked, as the form field text, to start another. Without one, there is no button.
	TryAgainURL string
	// Tab is the shell tab a Run of the kind belongs to, marked as current on its page.
	// Without one, the Runs tab.
	Tab web.Tab
	// About is what the Run's page tells under its header beyond the Run, such as the
	// Cycle it planned and what to do after it failed, or nothing.
	About func(ctx context.Context, run Run) (templ.Component, error)
	// Reasons are why a Run of the kind fails, as its Trace tells it, beside the reasons
	// every Run can fail for, such as agent.NoResult: the model ended without what the
	// Run is for. A %s in one is what the failure said more, verbatim.
	Reasons map[agent.Reason]web.Message
}

// Handlers serve the Runs page, a Run's page and its live Trace, and cancel a Run.
type Handlers struct {
	Engine *Engine
	// Kinds are what the pages tell of each kind of Run.
	Kinds []KindPage
	Clock clock.Clock
	Log   *slog.Logger
}

// page is what the pages tell of the kind of Run k: nothing, for a kind no feature
// tells of.
func (h Handlers) page(k Kind) KindPage {
	for _, p := range h.Kinds {
		if p.Kind == k {
			return p
		}
	}
	return KindPage{}
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
	kind := h.page(run.Kind)
	var res result
	if kind.ResultURL != nil && run.Status == Succeeded {
		res.Text = web.T(ctx, kind.ResultLabel)
		if res.Href, err = kind.ResultURL(ctx, id); err != nil {
			httpx.ServerError(w, r, h.Log, "find what the Run left", err)
			return
		}
	}
	// The page fetches itself as its Run ends: a Run that left something goes on to it.
	if r.URL.Query().Has("ended") && res.Href != "" {
		w.Header().Set("HX-Location", res.Href)
		return
	}
	var about templ.Component
	if kind.About != nil {
		if about, err = kind.About(ctx, run); err != nil {
			httpx.ServerError(w, r, h.Log, "tell what the Run is about", err)
			return
		}
	}
	again := ""
	if run.Status == Failed {
		again = kind.TryAgainURL
	}
	httpx.Render(w, r, h.Log, http.StatusOK, runPage(h.Clock.Now(), run, kind, res, again, about))
}

// List is the signed-in User's Runs, newest first.
func (h Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Engine.List(r.Context())
	if err != nil {
		httpx.ServerError(w, r, h.Log, "list the Runs", err)
		return
	}
	names := map[Kind]web.Message{}
	for _, k := range h.Kinds {
		names[k.Kind] = k.Name
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
	k, _, found, err := h.Engine.Status(ctx, id)
	if err != nil {
		httpx.ServerError(w, r, h.Log, "load the Run", err)
		return
	}
	kind := h.page(k)
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
		if err := traceEvent(ev, zone, kind).Render(ctx, &b); err != nil {
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
	_, status, _, err := h.Engine.Status(ctx, id)
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

// Failure is why run, of the kind k tells of, failed, in the Language of ctx, as its
// Trace tells it: "" for a Run that didn't.
func (k KindPage) Failure(ctx context.Context, run Run) string {
	for _, ev := range run.Trace {
		if ev.Kind == agent.Failed {
			return line(ctx, ev.TraceEvent, k)
		}
	}
	return ""
}
