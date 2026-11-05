package profile

import (
	"context"
	"errors"
	"maps"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

// loadLanguage reads the signed-in User's Language, if they have chosen one. It has no
// WHERE on purpose: run in db.AsUser, row-level security leaves only their row.
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

// loadSlotTimes reads the signed-in User's times of day of each kind of Slot: the
// defaults until they set theirs. It has no WHERE on purpose: run in db.AsUser,
// row-level security leaves only their row.
func loadSlotTimes(ctx context.Context, q db.Querier) (domain.SlotTimes, error) {
	var b, l, s, d pgtype.Time
	err := q.QueryRow(ctx, `SELECT breakfast_at, lunch_at, snack_at, dinner_at FROM profile`).Scan(&b, &l, &s, &d)
	if err != nil {
		return nil, err
	}
	if !b.Valid {
		return maps.Clone(domain.DefaultSlotTimes), nil
	}
	return domain.SlotTimes{
		domain.Breakfast: timeOfDay(b), domain.Lunch: timeOfDay(l),
		domain.Snack: timeOfDay(s), domain.Dinner: timeOfDay(d),
	}, nil
}

// timeOfDay is t to the minute.
func timeOfDay(t pgtype.Time) domain.TimeOfDay {
	return domain.TimeOfDay(time.Duration(t.Microseconds) * time.Microsecond / time.Minute)
}

// saveLanguage creates the signed-in User's Profile with Language l, or changes the
// Language of the one they have.
func saveLanguage(ctx context.Context, q db.Querier, l domain.Language) error {
	_, err := q.Exec(ctx, `
		INSERT INTO profile (language) VALUES ($1)
		ON CONFLICT (user_id) DO UPDATE SET language = excluded.language`,
		string(l))
	return err
}

// loadFoodTerms is the Food Terms the signed-in User has written: none yet, until a slice
// brings one.
func loadFoodTerms(context.Context, db.Querier) (domain.FoodTerms, error) {
	return domain.FoodTerms{}, nil
}
