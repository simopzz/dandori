package apptest

import (
	"bufio"
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// StartRun starts a Run of kind, as the app's forms do, and returns the
// path of its trace panel.
func (h *Harness) StartRun(t testing.TB, kind string) string {
	t.Helper()
	res := h.PostForm(t, "/runs", url.Values{"kind": {kind}})
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Path, "/runs/") {
		t.Fatalf("start %s Run = %d at %s, want its trace panel:\n%s", kind, res.StatusCode, res.Path, res.Body)
	}
	return res.Path
}

// Event is one server-sent event.
type Event struct {
	ID   string
	Name string
	Data string
}

// Stream is an open server-sent event stream.
type Stream struct {
	events chan Event
	cancel context.CancelFunc
}

// streamTimeout bounds how long a test waits for the next event.
const streamTimeout = 10 * time.Second

// Stream opens the server-sent event stream at path, sending lastEventID as
// a reconnecting browser does when it isn't empty.
func (h *Harness) Stream(t testing.TB, path, lastEventID string) *Stream {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	res, err := h.Client.Do(req) //nolint:bodyclose // the caller closes the stream
	if err != nil {
		cancel()
		t.Fatalf("GET %s: %v", path, err)
	}
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		res.Body.Close()
		cancel()
		t.Fatalf("GET %s = %d %s, want an event stream", path, res.StatusCode, res.Header.Get("Content-Type"))
	}

	s := &Stream{events: make(chan Event), cancel: cancel}
	t.Cleanup(cancel)
	go func() {
		defer close(s.events)
		defer res.Body.Close()
		var ev Event
		var data []string
		sc := bufio.NewScanner(res.Body)
		sc.Buffer(nil, 1<<20)
		for sc.Scan() {
			line := sc.Text()
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "":
				if line != "" {
					continue // a comment
				}
				if data == nil {
					continue
				}
				ev.Data = strings.Join(data, "\n")
				select {
				case s.events <- ev:
				case <-ctx.Done():
					return
				}
				ev, data = Event{}, nil
			case "id":
				ev.ID = value
			case "event":
				ev.Name = value
			case "data":
				data = append(data, value)
			}
		}
	}()
	return s
}

// Next returns the next event, or false once the server has ended the
// stream.
func (s *Stream) Next(t testing.TB) (Event, bool) {
	t.Helper()
	select {
	case ev, ok := <-s.events:
		return ev, ok
	case <-time.After(streamTimeout):
		t.Fatalf("no event within %s", streamTimeout)
		return Event{}, false
	}
}

// Rest reads every remaining event until the server ends the stream.
func (s *Stream) Rest(t testing.TB) []Event {
	t.Helper()
	var events []Event
	for {
		ev, ok := s.Next(t)
		if !ok {
			return events
		}
		events = append(events, ev)
	}
}

// Close hangs up, as a browser does when the page is closed.
func (s *Stream) Close() { s.cancel() }
