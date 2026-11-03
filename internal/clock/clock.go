// Package clock is the one place Dandori reads the time (ADR-0014): every
// other package is given a Clock, so tests can set the time and step it.
package clock

import (
	"sync"
	"time"
	_ "time/tzdata" // DefaultZone loads without a zone database, as on distroless
)

// DefaultZone is the zone a day starts and ends in unless
// DANDORI_TIMEZONE names another.
const DefaultZone = "Europe/Rome"

// Clock tells the time in the installation's zone.
type Clock interface {
	Now() time.Time
}

// Real is the machine's time, told in loc.
func Real(loc *time.Location) Clock { return real{loc} }

type real struct{ loc *time.Location }

func (r real) Now() time.Time { return time.Now().In(r.loc) }

// Manual is a clock that stands still until it is set, for tests.
type Manual struct {
	mu  sync.Mutex
	now time.Time
}

// NewManual returns a clock standing at now.
func NewManual(now time.Time) *Manual { return &Manual{now: now} }

// Now is the time the clock was last set to.
func (m *Manual) Now() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.now
}

// Set moves the clock to now.
func (m *Manual) Set(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}
