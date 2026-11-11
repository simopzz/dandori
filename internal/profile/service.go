// Package profile is the User's Profile: for now, their Language, chosen at their first
// sign-in and changeable until they have written a Food Term (ADR-0015), and the time of
// day of each kind of Slot.
package profile

import (
	"context"
	"errors"

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
