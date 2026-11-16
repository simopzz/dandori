// Package today is Today, the page a User opens Dandori on: for now, the day's Slot now
// or next by the Profile's times of day, the day's later ones, how up to date the kitchen
// is, and an offer to open a Cycle while there's none. Later slices fill each Slot in.
package today

import (
	"context"
	"time"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/profile"
)

// Service is what Today shows the signed-in User.
type Service struct {
	profile *profile.Service
	ledger  *ledger.Service
	noCycle NoCycle
}

// NoCycle reports whether the signed-in User has no Cycle at now, current or upcoming.
type NoCycle func(ctx context.Context, now time.Time) (bool, error)

// New creates the Service.
func New(profile *profile.Service, ledger *ledger.Service, noCycle NoCycle) *Service {
	return &Service{profile: profile, ledger: ledger, noCycle: noCycle}
}

// Day is what Today shows at a moment.
type Day struct {
	Slots domain.Today
	Times domain.SlotTimes
	// Beat is when the scheduler last brought the kitchen up to now; Beaten is false
	// until it first has.
	Beat   time.Time
	Beaten bool
	// NoCycle is whether the User has no Cycle, current or upcoming, to plan.
	NoCycle bool
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
	noCycle, err := s.noCycle(ctx, now)
	if err != nil {
		return Day{}, err
	}
	return Day{Slots: domain.TodayAt(now, times), Times: times, Beat: beat, Beaten: beaten, NoCycle: noCycle}, nil
}
