package runs

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/simopzz/dandori/internal/httpx"
)

// Handlers serve the Runs pages and the live Trace stream.
type Handlers struct {
	Engine *Engine
	Log    *slog.Logger
	// Demo offers demo Runs on the Runs page.
	Demo bool
}

func (h Handlers) Start(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	kind := r.PostForm.Get("kind")
	newJob, ok := h.Engine.job(kind)
	if !ok {
		http.Error(w, fmt.Sprintf("There is no kind of Run called %q.", kind), http.StatusBadRequest)
		return
	}
	job, err := newJob(r.PostForm)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := h.Engine.Start(r.Context(), kind, job)
	if err != nil {
		httpx.ServerError(w, h.Log, "start run", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
}

func (h Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Engine.List(r.Context())
	if err != nil {
		httpx.ServerError(w, h.Log, "list runs", err)
		return
	}
	templ.Handler(RunsPage(list, h.Demo)).ServeHTTP(w, r)
}

func (h Handlers) Show(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	run, found, err := h.Engine.Get(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, h.Log, "load run", err)
		return
	}
	if !found {
		http.NotFound(w, r)
		return
	}
	run.Events, err = h.Engine.Trace(r.Context(), id, 0)
	if err != nil {
		httpx.ServerError(w, h.Log, "load trace", err)
		return
	}
	templ.Handler(RunPage(run)).ServeHTTP(w, r)
}

func (h Handlers) Cancel(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	if h.Engine.Cancel(id) {
		http.Redirect(w, r, fmt.Sprintf("/runs/%d", id), http.StatusSeeOther)
		return
	}
	_, found, err := h.Engine.Get(r.Context(), id)
	switch {
	case err != nil:
		httpx.ServerError(w, h.Log, "load run", err)
	case !found:
		http.NotFound(w, r)
	default:
		http.Error(w, "This Run has already ended.", http.StatusConflict)
	}
}

// Stream sends a Run's Trace Events as server-sent events: the stored ones
// after the last the client has (Last-Event-ID, else ?after=), then live
// ones until the Run ends, then an "end" event with its status. Trace
// Events are unnamed messages, the only ones htmx's hx-sse swaps in.
func (h Handlers) Stream(w http.ResponseWriter, r *http.Request) {
	id, ok := httpx.PathID(w, r)
	if !ok {
		return
	}
	run, found, err := h.Engine.Get(r.Context(), id)
	if err != nil {
		httpx.ServerError(w, h.Log, "load run", err)
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

	// Follow before reading the stored trace, so no event falls between.
	var live chan TraceEvent
	lr := h.Engine.get(id)
	if lr != nil {
		if ch, ok := lr.subscribe(); ok {
			live = ch
			defer lr.unsubscribe(ch)
		}
	}
	stored, err := h.Engine.Trace(r.Context(), id, after)
	if err != nil {
		httpx.ServerError(w, h.Log, "load trace", err)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher := http.NewResponseController(w)
	send := func(ev TraceEvent) error {
		var html bytes.Buffer
		if err := TraceEventItem(ev).Render(r.Context(), &html); err != nil {
			return err
		}
		writeEvent(w, strconv.Itoa(ev.Seq), "", html.String())
		after = ev.Seq
		return nil
	}
	for _, ev := range stored {
		if err := send(ev); err != nil {
			h.Log.Error("render trace event", "err", err)
			return
		}
	}
	if err := flusher.Flush(); err != nil {
		return
	}

	for live != nil {
		select {
		case ev, open := <-live:
			if !open {
				// The Run ended, or we were dropped for being slow; either
				// way, catch up from the stored trace.
				ended := lr.ended()
				missed, err := h.Engine.Trace(r.Context(), id, after)
				if err != nil {
					h.Log.Error("load trace", "err", err)
					return
				}
				for _, ev := range missed {
					if err := send(ev); err != nil {
						h.Log.Error("render trace event", "err", err)
						return
					}
				}
				if !ended {
					flusher.Flush()
					return // the browser reconnects and follows on
				}
				live = nil
				continue
			}
			if ev.Seq <= after {
				continue
			}
			if err := send(ev); err != nil {
				h.Log.Error("render trace event", "err", err)
				return
			}
			if err := flusher.Flush(); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
	run, _, err = h.Engine.Get(r.Context(), id)
	if err != nil {
		h.Log.Error("load run", "err", err)
		return
	}
	if run.Status == "running" {
		// Nothing here is running it, but its end was never stored (say the
		// database dropped). Hang up without an end, so the panel doesn't
		// reload into the same state; the browser retries later.
		return
	}
	writeEvent(w, "", "end", run.Status)
	flusher.Flush()
}

// writeEvent writes one server-sent event; one without a name is a
// message.
func writeEvent(w io.Writer, id, name, data string) {
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
