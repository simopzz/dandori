package runs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/web"
)

// liveRun is a Run in progress. It is the Run's trace sink: every Trace
// Event is stored with the Run and fanned out to the live streams.
type liveRun struct {
	id     int64
	cancel context.CancelFunc
	q      db.Querier
	log    *slog.Logger

	mu   sync.Mutex
	seq  int
	subs map[chan web.TraceEvent]struct{}
	done bool
}

// Emit implements agent.Trace.
func (r *liveRun) Emit(e agent.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	ev := web.TraceEvent{
		Seq: r.seq,
		// Postgres keeps microseconds; so does the live copy.
		At:      time.Now().Truncate(time.Microsecond),
		Kind:    string(e.Kind),
		Summary: e.Summary,
		Detail:  e.Detail,
	}
	// Not the Run's context: a cancelled Run still records how it ended.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := insertEvent(ctx, r.q, r.id, ev); err != nil {
		r.log.Error("store trace event", "run", r.id, "seq", ev.Seq, "err", err)
	}
	for ch := range r.subs {
		select {
		case ch <- ev:
		default:
			// Too slow to keep up: hang up, and let the browser reconnect
			// and catch up from the stored trace.
			delete(r.subs, ch)
			close(ch)
		}
	}
}

// subscribe follows the Run's Trace Events from now on. It returns false
// once the Run has ended.
func (r *liveRun) subscribe() (chan web.TraceEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.done {
		return nil, false
	}
	ch := make(chan web.TraceEvent, 64)
	r.subs[ch] = struct{}{}
	return ch, true
}

func (r *liveRun) unsubscribe(ch chan web.TraceEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.subs[ch]; ok {
		delete(r.subs, ch)
		close(ch)
	}
}

func (r *liveRun) ended() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.done
}

// finish ends every live stream of the Run.
func (r *liveRun) finish() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.done = true
	for ch := range r.subs {
		delete(r.subs, ch)
		close(ch)
	}
}
