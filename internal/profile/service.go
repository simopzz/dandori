// Package profile is the user's Profile: their Targets, dietary exclusions
// and allergies, Meal Out estimate, Untracked Allowance and Slot times;
// and their Planning Brief, shown beside it.
package profile

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/domain"
)

// InvalidError means a Profile breaks the domain rules. Fields is what is
// wrong, keyed by domain field path as domain.Profile.Validate gives it.
type InvalidError struct{ Fields map[string]string }

func (e *InvalidError) Error() string { return "invalid profile" }

// Service is the Profile's use cases.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Get loads the Profile. found is false until it is first saved.
func (s *Service) Get(ctx context.Context) (p domain.Profile, found bool, err error) {
	return loadProfile(ctx, s.pool)
}

// Save stores p as the Profile.
func (s *Service) Save(ctx context.Context, p domain.Profile) error {
	if invalid := p.Validate(); len(invalid) > 0 {
		return &InvalidError{Fields: invalid}
	}
	return saveProfile(ctx, s.pool, p)
}

// Brief loads the Planning Brief: the user's lasting wishes for how their
// meals are planned, empty until written.
func (s *Service) Brief(ctx context.Context) (string, error) {
	return loadBrief(ctx, s.pool)
}

// SaveBrief stores brief as the Planning Brief. It applies from the next
// planning Run: no Run starts, and no Plan needs a new Plan Proposal.
func (s *Service) SaveBrief(ctx context.Context, brief string) error {
	return saveBrief(ctx, s.pool, strings.TrimSpace(brief))
}
