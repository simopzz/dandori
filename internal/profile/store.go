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
	"github.com/simopzz/dandori/internal/ingredients"
	"github.com/simopzz/dandori/internal/ledger"
	"github.com/simopzz/dandori/internal/recipes"
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

// loadTargets reads the signed-in User's Targets; set is false until they save theirs,
// when only the tolerance is. It has no WHERE on purpose: run in db.AsUser, row-level
// security leaves only their row.
func loadTargets(ctx context.Context, q db.Querier) (t domain.Targets, set bool, err error) {
	var kcal, protein, fibre, sugar pgtype.Int4
	err = q.QueryRow(ctx, `SELECT target_kcal, target_protein, target_fibre, target_sugar, kcal_tolerance FROM profile`).
		Scan(&kcal, &protein, &fibre, &sugar, &t.Tolerance)
	if err != nil {
		return domain.Targets{}, false, err
	}
	t.Kcal, t.Protein, t.Fibre, t.Sugar = int(kcal.Int32), int(protein.Int32), int(fibre.Int32), int(sugar.Int32)
	return t, kcal.Valid, nil
}

// saveTargets sets the signed-in User's Targets to t.
func saveTargets(ctx context.Context, q db.Querier, t domain.Targets) error {
	_, err := q.Exec(ctx, `
		UPDATE profile SET target_kcal = $1, target_protein = $2, target_fibre = $3, target_sugar = $4, kcal_tolerance = $5`,
		t.Kcal, t.Protein, t.Fibre, t.Sugar, t.Tolerance)
	return err
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

// countParsing is how many parse Runs the signed-in User has going.
func countParsing(ctx context.Context, q db.Querier) (int, error) {
	return recipes.CountParsing(ctx, q)
}

// loadFoodTerms is the signed-in User's Food Terms: Ingredients, Portions, Recipes and
// Recipe Drafts.
func loadFoodTerms(ctx context.Context, q db.Querier) (domain.FoodTerms, error) {
	ings, err := ingredients.CountIngredients(ctx, q)
	if err != nil {
		return domain.FoodTerms{}, err
	}
	portions, err := ledger.CountPortions(ctx, q)
	if err != nil {
		return domain.FoodTerms{}, err
	}
	recs, err := recipes.CountRecipes(ctx, q)
	if err != nil {
		return domain.FoodTerms{}, err
	}
	drafts, err := recipes.CountDrafts(ctx, q)
	return domain.FoodTerms{Ingredients: ings, Portions: portions, Recipes: recs, Drafts: drafts}, err
}
