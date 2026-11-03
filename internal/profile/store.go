package profile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// loadLanguage reads the signed-in User's Language, if they have chosen
// one. It has no WHERE on purpose: run in db.AsUser, row-level security
// leaves only their row.
func loadLanguage(ctx context.Context, q db.Querier) (domain.Language, bool, error) {
	var code string
	err := q.QueryRow(ctx, `SELECT language FROM profile`).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return domain.Language(code), true, nil
}

// saveLanguage creates the signed-in User's Profile with Language l, or
// changes the Language of the one they have.
func saveLanguage(ctx context.Context, q db.Querier, l domain.Language) error {
	_, err := q.Exec(ctx, `
		INSERT INTO profile (language) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET language = excluded.language`,
		string(l))
	return err
}

// loadKitchen is what the signed-in User's kitchen holds, as far as the
// Language rule needs it: nothing yet, until a slice brings a food term.
func loadKitchen(context.Context, db.Querier) (domain.Kitchen, error) {
	return domain.Kitchen{}, nil
}
