// Package today is Today, the page a User opens Dandori on: for now, the day's Slot now
// or next by the Profile's times of day, the day's later ones, and how up to date the
// kitchen is. Later slices fill each Slot in.
package today

import (
	"context"
	"time"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/kitchen"
	"github.com/simopzz/dandori/internal/profile"
)

// Service is what Today shows the signed-in User.
type Service struct {
	profile *profile.Service
	kitchen *kitchen.Service
}

// New creates the Service.
func New(profile *profile.Service, kitchen *kitchen.Service) *Service {
	return &Service{profile: profile, kitchen: kitchen}
}

// Day is what Today shows at a moment.
type Day struct {
	Slots domain.Today
	Times domain.SlotTimes
	// Beat is when the scheduler last brought the kitchen up to now; Beaten is false
	// until it first has.
	Beat   time.Time
	Beaten bool
}

// At is what Today shows the signed-in User at now.
func (s *Service) At(ctx context.Context, now time.Time) (Day, error) {
	times, err := s.profile.SlotTimes(ctx)
	if err != nil {
		return Day{}, err
	}
	beat, beaten, err := s.kitchen.LastBeat(ctx)
	if err != nil {
		return Day{}, err
	}
	return Day{Slots: domain.TodayAt(now, times), Times: times, Beat: beat, Beaten: beaten}, nil
}
