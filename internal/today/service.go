// Package today is Today, the page a User opens Dandori on: for now, the day's Slot now
// or next by the Profile's times of day, the day's later ones, how up to date the kitchen
// is, and an offer to open a Cycle while there's none. Later slices fill each Slot in.
package today

import (
	"context"
	"time"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/planning"
	"github.com/simopzz/dandori/internal/profile"
)

// Service is what Today shows the signed-in User.
type Service struct {
	profile  *profile.Service
	ledger   *ledger.Service
	planning *planning.Service
}

// New creates the Service.
func New(profile *profile.Service, ledger *ledger.Service, planning *planning.Service) *Service {
	return &Service{profile: profile, ledger: ledger, planning: planning}
}

// Day is what Today shows at a moment.
type Day struct {
	Slots domain.Today
	Times domain.SlotTimes
	// Beat is when the scheduler last brought the kitchen up to now; Beaten is false
	// until it first has.
	Beat   time.Time
	Beaten bool
	// Planning are the User's current and upcoming Cycles still to plan, in that order,
	// each as Today reminds them of it; a Cycle with a Plan, and nothing pending or
	// going, has none.
	Planning []Reminder
	// Offer is what Today offers about opening a Cycle.
	Offer Offer
}

// Reminder is a Cycle Today reminds the User of planning: its id, whether it is the
// next Cycle, upcoming, or the current one, and how planning it stands.
type Reminder struct {
	CycleID  int64
	Next     bool
	Standing planning.Standing
}

// Offer is what Today offers about opening a Cycle.
type Offer int

const (
	// NoOffer is offering nothing: a Cycle is upcoming, or the current one ends later
	// than openNextDays from now.
	NoOffer Offer = iota
	// OpenFirst is offering to open a Cycle, while the User has none current or upcoming.
	OpenFirst
	// OpenNext is offering to open the next Cycle, from openNextDays before the current
	// one's end date.
	OpenNext
	// TargetsFirst is pointing to the Profile instead, while the Targets aren't set:
	// opening would be refused.
	TargetsFirst
)

// openNextDays is how many days before the current Cycle's end date Today says to open
// the next, so it is planned and ordered in time.
const openNextDays = 3

// offer is what Today offers at now about opening a Cycle, with the User's current and
// upcoming Cycles, each nil when they have none.
func offer(current, upcoming *planning.Cycle, now time.Time) Offer {
	switch {
	case upcoming != nil:
		return NoOffer
	case current == nil:
		return OpenFirst
	case !now.Before(current.Ends.AddDate(0, 0, -openNextDays)):
		return OpenNext
	}
	return NoOffer
}

// At is what Today shows the signed-in User at now.
func (s *Service) At(ctx context.Context, now time.Time) (Day, error) {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return Day{}, err
	}
	beat, beaten, err := s.ledger.LastBeat(ctx)
	if err != nil {
		return Day{}, err
	}
	current, upcoming, err := s.planning.Now(ctx, now)
	if err != nil {
		return Day{}, err
	}
	d := Day{
		Slots: domain.TodayAt(now, times), Times: times, Beat: beat, Beaten: beaten,
		Offer: offer(current, upcoming, now),
	}
	if d.Offer != NoOffer {
		_, set, err := s.profile.Targets(ctx)
		if err != nil {
			return Day{}, err
		}
		if !set {
			d.Offer = TargetsFirst
		}
	}
	for _, c := range []*planning.Cycle{current, upcoming} {
		if c == nil {
			continue
		}
		st, err := s.planning.StandingOf(ctx, c.ID)
		if err != nil {
			return Day{}, err
		}
		if st != planning.Planned {
			d.Planning = append(d.Planning, Reminder{CycleID: c.ID, Next: c == upcoming, Standing: st})
		}
	}
	return d, nil
}
