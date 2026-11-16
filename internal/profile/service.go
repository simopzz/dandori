// Package profile is the User's Profile: for now, their Language, chosen at their first
// sign-in and changeable until they have written a Food Term (ADR-0015), the time of day
// of each kind of Slot, their Targets, their Meal Out estimate, their exclusions and
// allergies, the appliances they own, and their Planning Brief.
package profile

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// ErrLanguageStays is a change of Language once the User has written a Food Term.
var ErrLanguageStays = errors.New("the User has written Food Terms in the Language, so it stays")

// ErrParsing is a change of Language while a parse Run is going, whose Recipe Draft would
// arrive in the Language left.
var ErrParsing = errors.New("a parse Run is going in the Language, so it stays")

// Service is the signed-in User's Profile.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Language is the signed-in User's Language; ok is false until they choose one.
func (s *Service) Language(ctx context.Context) (l domain.Language, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		l, ok, err = loadLanguage(ctx, tx)
		return err
	})
	return l, ok, err
}

// SlotTimes is the signed-in User's time of day of each kind of Slot, which they have
// once they have chosen their Language.
func (s *Service) SlotTimes(ctx context.Context) (t domain.SlotTimes, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		t, err = loadSlotTimes(ctx, tx)
		return err
	})
	return t, err
}

// SetSlotTimes saves typed, by SlotKind, as the signed-in User's time of day of each
// kind of Slot, unless a field is refused: then it saves nothing and is each problem.
func (s *Service) SetSlotTimes(ctx context.Context, typed map[string]string) (domain.SlotTimes, map[string]domain.Problem, error) {
	t, problems := domain.ParseSlotTimes(typed)
	if len(problems) > 0 {
		return t, problems, nil
	}
	return t, nil, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveSlotTimes(ctx, tx, t) })
}

// Targets is the signed-in User's Targets; set is false until they save theirs, when
// only the tolerance is.
func (s *Service) Targets(ctx context.Context) (t domain.Targets, set bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		t, set, err = LoadTargets(ctx, tx)
		return err
	})
	return t, set, err
}

// SetTargets saves typed as the signed-in User's Targets, by field, unless a field is
// refused: then it saves nothing and is the problem with each.
func (s *Service) SetTargets(ctx context.Context, typed map[string]string) (domain.Targets, map[string]domain.Problem, error) {
	t, problems := domain.ParseTargets(typed)
	if len(problems) > 0 {
		return t, problems, nil
	}
	return t, nil, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveTargets(ctx, tx, t) })
}

// MealOut is the signed-in User's Meal Out estimate: what a Meal Out counts unless its
// Exception gives its own.
func (s *Service) MealOut(ctx context.Context) (n domain.Nutrition, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		n, err = loadMealOut(ctx, tx)
		return err
	})
	return n, err
}

// SetMealOut saves typed as the signed-in User's Meal Out estimate, by field, unless a
// field is refused: then it saves nothing and is the problem with each.
func (s *Service) SetMealOut(ctx context.Context, typed map[string]string) (domain.Nutrition, map[string]domain.Problem, error) {
	n, problems := domain.ParseMealOut(typed)
	if len(problems) > 0 {
		return n, problems, nil
	}
	return n, nil, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveMealOut(ctx, tx, n) })
}

// Foods are the signed-in User's exclusions and allergies, the foods they never eat and
// those they are allergic to, each normalised.
func (s *Service) Foods(ctx context.Context) (exclusions, allergies []string, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		exclusions, allergies, err = loadFoods(ctx, tx)
		return err
	})
	return exclusions, allergies, err
}

// SetFoods saves the exclusions and allergies typed as the signed-in User's, each
// normalised, and is what it saved. Either may be empty.
func (s *Service) SetFoods(ctx context.Context, typedExclusions, typedAllergies string) (exclusions, allergies []string, err error) {
	exclusions, allergies = domain.NormaliseFoods(typedExclusions), domain.NormaliseFoods(typedAllergies)
	return exclusions, allergies, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveFoods(ctx, tx, exclusions, allergies) })
}

// Brief is the signed-in User's Planning Brief, empty until they write one.
func (s *Service) Brief(ctx context.Context) (brief string, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		brief, err = loadBrief(ctx, tx)
		return err
	})
	return brief, err
}

// SetBrief saves typed as the signed-in User's Planning Brief, and is what it saved:
// each line ending in a bare newline, and without the blank space around it, so a blank
// Brief is empty.
func (s *Service) SetBrief(ctx context.Context, typed string) (string, error) {
	brief := strings.TrimSpace(strings.ReplaceAll(typed, "\r\n", "\n"))
	return brief, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveBrief(ctx, tx, brief) })
}

// Appliances are the kinds of appliance the signed-in User owns, each with their note on
// its model: a hob and an oven until they say otherwise.
func (s *Service) Appliances(ctx context.Context) (owned domain.Appliances, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		owned, err = loadAppliances(ctx, tx)
		return err
	})
	return owned, err
}

// SetAppliances saves the kinds whose codes are ticked as those the signed-in User owns,
// each with its note in notes, and is what it saved: a note on a kind left unticked
// is dropped.
func (s *Service) SetAppliances(ctx context.Context, ticked []string, notes map[domain.ApplianceKind]string) (domain.Appliances, error) {
	owned := domain.ParseAppliances(ticked, notes)
	return owned, db.AsUser(ctx, s.pool, func(tx pgx.Tx) error { return saveAppliances(ctx, tx, owned) })
}

// SetLanguage chooses the signed-in User's Language, or changes it while they have
// written no Food Term; after that it is ErrLanguageStays, and while a parse Run is going
// it is ErrParsing.
func (s *Service) SetLanguage(ctx context.Context, l domain.Language) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		current, chosen, err := loadLanguage(ctx, tx)
		if err != nil {
			return err
		}
		if chosen && current != l {
			terms, err := loadFoodTerms(ctx, tx)
			if err != nil {
				return err
			}
			if !domain.LanguageCanChange(terms) {
				return ErrLanguageStays
			}
			parsing, err := countParsing(ctx, tx)
			if err != nil {
				return err
			}
			if parsing > 0 {
				return ErrParsing
			}
		}
		return saveLanguage(ctx, tx, l)
	})
}
