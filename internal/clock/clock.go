// Package clock is the one place Dandori reads the time (ADR-0014): every
// other package is given a Clock, so tests can set the time and step it.
package clock

import (
	"context"
	"sync"
	"time"
	_ "time/tzdata" // DefaultZone loads without a zone database, as on distroless
)

// DefaultZone is the zone a day starts and ends in unless
// DANDORI_TIMEZONE names another.
const DefaultZone = "Europe/Rome"

// Clock tells the time in the installation's zone, and beats on it.
type Clock interface {
	Now() time.Time
	// Every calls beat with the time each time the clock passes a multiple of d (14:00,
	// 14:05 for five minutes), from now until ctx ends. A clock that passes several at
	// once, as a machine waking up does, beats once. It returns at once.
	Every(ctx context.Context, d time.Duration, beat func(now time.Time))
}

// Real is the machine's time, told in loc.
func Real(loc *time.Location) Clock { return real{loc} }

type real struct{ loc *time.Location }

func (r real) Now() time.Time { return time.Now().In(r.loc) }

func (r real) Every(ctx context.Context, d time.Duration, beat func(now time.Time)) {
	go func() {
		for {
			timer := time.NewTimer(time.Until(r.Now().Truncate(d).Add(d)))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				beat(r.Now())
			}
		}
	}()
}

// Manual is a clock that stands still until it is set, for tests. Setting it past a
// multiple of a beat's interval beats before Set returns, so a test that sets it never
// waits for the beat.
type Manual struct {
	mu    sync.Mutex
	now   time.Time
	beats []manualBeat
}

type manualBeat struct {
	ctx  context.Context
	d    time.Duration
	beat func(now time.Time)
}

// NewManual returns a clock standing at now.
func NewManual(now time.Time) *Manual { return &Manual{now: now} }

// Now is the time the clock was last set to.
func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Set moves the clock to now, and beats each beat whose interval has a multiple since the
// time it stood at, up to now.
func (m *Manual) Set(now time.Time) {
	m.mu.Lock()
	was := m.now
	m.now = now
	var due []func(time.Time)
	for _, b := range m.beats {
		if b.ctx.Err() == nil && now.Truncate(b.d).After(was) {
			due = append(due, b.beat)
		}
	}
	m.mu.Unlock()
	for _, beat := range due {
		beat(now)
	}
}

// Every beats beat whenever Set passes a multiple of d, until ctx ends.
func (m *Manual) Every(ctx context.Context, d time.Duration, beat func(now time.Time)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beats = append(m.beats, manualBeat{ctx, d, beat})
}
