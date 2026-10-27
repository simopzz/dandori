package profile

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/simopzz/dandori/internal/db"
	"github.com/simopzz/dandori/internal/domain"
)

func loadProfile(ctx context.Context, q db.Querier) (domain.Profile, bool, error) {
	var p domain.Profile
	var breakfast, lunch, snack, dinner domain.TimeOfDay
	err := q.QueryRow(ctx, `
		SELECT target_kcal, target_protein_g, target_carbs_g, target_fat_g,
		       exclusions, allergies,
		       meal_out_kcal, meal_out_protein_g, meal_out_carbs_g, meal_out_fat_g,
		       untracked_kcal, untracked_protein_g, untracked_carbs_g, untracked_fat_g,
		       `+db.Minutes("breakfast_at")+`, `+db.Minutes("lunch_at")+`, `+db.Minutes("snack_at")+`, `+db.Minutes("dinner_at")+`
		FROM profile`).Scan(
		&p.Targets.Kcal, &p.Targets.ProteinG, &p.Targets.CarbsG, &p.Targets.FatG,
		&p.Exclusions, &p.Allergies,
		&p.MealOut.Kcal, &p.MealOut.ProteinG, &p.MealOut.CarbsG, &p.MealOut.FatG,
		&p.UntrackedAllowance.Kcal, &p.UntrackedAllowance.ProteinG, &p.UntrackedAllowance.CarbsG, &p.UntrackedAllowance.FatG,
		&breakfast, &lunch, &snack, &dinner,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{SlotTimes: domain.DefaultSlotTimes}, false, nil
	}
	if err != nil {
		return domain.Profile{}, false, err
	}
	p.SlotTimes = domain.SlotTimes{domain.Breakfast: breakfast, domain.Lunch: lunch, domain.Snack: snack, domain.Dinner: dinner}
	return p, true, nil
}

func saveProfile(ctx context.Context, q db.Querier, p domain.Profile) error {
	_, err := q.Exec(ctx, `
		INSERT INTO profile (
			target_kcal, target_protein_g, target_carbs_g, target_fat_g,
			exclusions, allergies,
			meal_out_kcal, meal_out_protein_g, meal_out_carbs_g, meal_out_fat_g,
			untracked_kcal, untracked_protein_g, untracked_carbs_g, untracked_fat_g,
			breakfast_at, lunch_at, snack_at, dinner_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14,
			`+db.Time("$15")+`, `+db.Time("$16")+`, `+db.Time("$17")+`, `+db.Time("$18")+`)
		ON CONFLICT (id) DO UPDATE SET
			target_kcal = EXCLUDED.target_kcal,
			target_protein_g = EXCLUDED.target_protein_g,
			target_carbs_g = EXCLUDED.target_carbs_g,
			target_fat_g = EXCLUDED.target_fat_g,
			exclusions = EXCLUDED.exclusions,
			allergies = EXCLUDED.allergies,
			meal_out_kcal = EXCLUDED.meal_out_kcal,
			meal_out_protein_g = EXCLUDED.meal_out_protein_g,
			meal_out_carbs_g = EXCLUDED.meal_out_carbs_g,
			meal_out_fat_g = EXCLUDED.meal_out_fat_g,
			untracked_kcal = EXCLUDED.untracked_kcal,
			untracked_protein_g = EXCLUDED.untracked_protein_g,
			untracked_carbs_g = EXCLUDED.untracked_carbs_g,
			untracked_fat_g = EXCLUDED.untracked_fat_g,
			breakfast_at = EXCLUDED.breakfast_at,
			lunch_at = EXCLUDED.lunch_at,
			snack_at = EXCLUDED.snack_at,
			dinner_at = EXCLUDED.dinner_at,
			updated_at = now()`,
		p.Targets.Kcal, p.Targets.ProteinG, p.Targets.CarbsG, p.Targets.FatG,
		p.Exclusions, p.Allergies,
		p.MealOut.Kcal, p.MealOut.ProteinG, p.MealOut.CarbsG, p.MealOut.FatG,
		p.UntrackedAllowance.Kcal, p.UntrackedAllowance.ProteinG, p.UntrackedAllowance.CarbsG, p.UntrackedAllowance.FatG,
		p.SlotTimes[domain.Breakfast], p.SlotTimes[domain.Lunch], p.SlotTimes[domain.Snack], p.SlotTimes[domain.Dinner],
	)
	return err
}
