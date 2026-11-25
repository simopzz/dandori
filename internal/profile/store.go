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

// LoadSlotTimes reads the signed-in User's times of day of each kind of Slot: the
// defaults until they set theirs. It has no WHERE on purpose: run in db.AsUser, row-level
// security leaves only their row.
func LoadSlotTimes(ctx context.Context, q db.Querier) (domain.SlotTimes, error) {
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

// saveSlotTimes sets the signed-in User's times of day of each kind of Slot to `t`.
func saveSlotTimes(ctx context.Context, q db.Querier, t domain.SlotTimes) error {
	_, err := q.Exec(ctx, `UPDATE profile SET breakfast_at = $1, lunch_at = $2, snack_at = $3, dinner_at = $4`,
		t[domain.Breakfast].String(), t[domain.Lunch].String(), t[domain.Snack].String(), t[domain.Dinner].String())
	return err
}

// timeOfDay is `t` to the minute.
func timeOfDay(t pgtype.Time) domain.TimeOfDay {
	return domain.TimeOfDay(time.Duration(t.Microseconds) * time.Microsecond / time.Minute)
}

// LoadTargets reads the signed-in User's Targets; `set` is false until they save theirs,
// when only the tolerance is. It has no WHERE on purpose: run in db.AsUser, row-level
// security leaves only their row.
func LoadTargets(ctx context.Context, q db.Querier) (t domain.Targets, set bool, err error) {
	var kcal, protein, fibre, sugar pgtype.Int4
	err = q.QueryRow(ctx, `SELECT target_kcal, target_protein, target_fibre, target_sugar, kcal_tolerance FROM profile`).
		Scan(&kcal, &protein, &fibre, &sugar, &t.Tolerance)
	if err != nil {
		return domain.Targets{}, false, err
	}
	t.Kcal, t.Protein, t.Fibre, t.Sugar = int(kcal.Int32), int(protein.Int32), int(fibre.Int32), int(sugar.Int32)
	return t, kcal.Valid, nil
}

// saveTargets sets the signed-in User's Targets to `t`.
func saveTargets(ctx context.Context, q db.Querier, t domain.Targets) error {
	_, err := q.Exec(ctx, `
		UPDATE profile SET target_kcal = $1, target_protein = $2, target_fibre = $3, target_sugar = $4, kcal_tolerance = $5`,
		t.Kcal, t.Protein, t.Fibre, t.Sugar, t.Tolerance)
	return err
}

// loadMealOut reads the signed-in User's Meal Out estimate: the default until they set
// theirs. It has no WHERE on purpose: run in db.AsUser, row-level security leaves only
// their row.
func loadMealOut(ctx context.Context, q db.Querier) (domain.Nutrition, error) {
	var kcal, protein, carbs, fat, fibre, sugar pgtype.Int4
	err := q.QueryRow(ctx, `
		SELECT meal_out_kcal, meal_out_protein, meal_out_carbs, meal_out_fat, meal_out_fibre, meal_out_sugar
		FROM profile`).Scan(&kcal, &protein, &carbs, &fat, &fibre, &sugar)
	if err != nil {
		return domain.Nutrition{}, err
	}
	amount := func(v pgtype.Int4) domain.Amount {
		if !v.Valid {
			return domain.Unknown
		}
		return domain.Known(float64(v.Int32))
	}
	return domain.Nutrition{
		Kcal: amount(kcal), Protein: amount(protein), Carbs: amount(carbs),
		Fat: amount(fat), Fibre: amount(fibre), Sugar: amount(sugar),
	}, nil
}

// saveMealOut sets the signed-in User's Meal Out estimate to `n`, its values whole
// numbers; an Unknown one is saved as null.
func saveMealOut(ctx context.Context, q db.Querier, n domain.Nutrition) error {
	whole := func(a domain.Amount) pgtype.Int4 { return pgtype.Int4{Int32: int32(a.Value), Valid: a.Known} }
	_, err := q.Exec(ctx, `
		UPDATE profile SET meal_out_kcal = $1, meal_out_protein = $2, meal_out_carbs = $3,
			meal_out_fat = $4, meal_out_fibre = $5, meal_out_sugar = $6`,
		whole(n.Kcal), whole(n.Protein), whole(n.Carbs), whole(n.Fat), whole(n.Fibre), whole(n.Sugar))
	return err
}

// loadFoods reads the signed-in User's `exclusions` and `allergies`, each normalised. It
// has no WHERE on purpose: run in db.AsUser, row-level security leaves only their row.
func loadFoods(ctx context.Context, q db.Querier) (exclusions, allergies []string, err error) {
	err = q.QueryRow(ctx, `SELECT exclusions, allergies FROM profile`).Scan(&exclusions, &allergies)
	return exclusions, allergies, err
}

// saveFoods sets the signed-in User's `exclusions` and `allergies`, each normalised.
func saveFoods(ctx context.Context, q db.Querier, exclusions, allergies []string) error {
	_, err := q.Exec(ctx, `UPDATE profile SET exclusions = $1, allergies = $2`, exclusions, allergies)
	return err
}

// loadBrief reads the signed-in User's Planning Brief, empty until they write one. It has
// no WHERE on purpose: run in db.AsUser, row-level security leaves only their row.
func loadBrief(ctx context.Context, q db.Querier) (brief string, err error) {
	err = q.QueryRow(ctx, `SELECT planning_brief FROM profile`).Scan(&brief)
	return brief, err
}

// saveBrief sets the signed-in User's Planning Brief to `brief`.
func saveBrief(ctx context.Context, q db.Querier, brief string) error {
	_, err := q.Exec(ctx, `UPDATE profile SET planning_brief = $1`, brief)
	return err
}

// loadAppliances reads the kinds of appliance the signed-in User owns, each with its
// note. It has no WHERE on purpose: run in db.AsUser, row-level security leaves only
// their row.
func loadAppliances(ctx context.Context, q db.Querier) (owned domain.Appliances, err error) {
	err = q.QueryRow(ctx, `SELECT appliances FROM profile`).Scan(&owned)
	return owned, err
}

// saveAppliances sets the kinds of appliance the signed-in User owns, each with its note.
func saveAppliances(ctx context.Context, q db.Querier, owned domain.Appliances) error {
	_, err := q.Exec(ctx, `UPDATE profile SET appliances = $1`, owned)
	return err
}

// saveLanguage creates the signed-in User's Profile with Language `l`, or changes the
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

// loadFoodTerms is the signed-in User's Food Terms: Ingredients, Portions, Recipes,
// Recipe Drafts, and the Profile's exclusions and allergies.
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
	if err != nil {
		return domain.FoodTerms{}, err
	}
	exclusions, allergies, err := loadFoods(ctx, q)
	return domain.FoodTerms{
		Ingredients: ings, Portions: portions, Recipes: recs, Drafts: drafts,
		Exclusions: len(exclusions), Allergies: len(allergies),
	}, err
}
