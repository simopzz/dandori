// Package profile is the User's Profile: for now, their Language, chosen
// at their first sign-in and changeable until their kitchen holds a food
// term (ADR-0015).
package profile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// ErrLanguageStays is a change of Language once the kitchen holds a food
// term.
var ErrLanguageStays = errors.New("the kitchen holds food terms in the Language, so it stays")

// Service is the signed-in User's Profile.
type Service struct {
	pool *pgxpool.Pool
}

// New creates the Service.
func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Language is the signed-in User's Language; ok is false until they
// choose one.
func (s *Service) Language(ctx context.Context) (l domain.Language, ok bool, err error) {
	err = db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		l, ok, err = loadLanguage(ctx, tx)
		return err
	})
	return l, ok, err
}

// SetLanguage chooses the signed-in User's Language, or changes it while
// their kitchen holds no food term; after that it is ErrLanguageStays.
func (s *Service) SetLanguage(ctx context.Context, l domain.Language) error {
	return db.AsUser(ctx, s.pool, func(tx pgx.Tx) error {
		current, chosen, err := loadLanguage(ctx, tx)
		if err != nil {
			return err
		}
		if chosen && current != l {
			kitchen, err := loadKitchen(ctx, tx)
			if err != nil {
				return err
			}
			if !domain.LanguageCanChange(kitchen) {
				return ErrLanguageStays
			}
		}
		return saveLanguage(ctx, tx, l)
	})
}
