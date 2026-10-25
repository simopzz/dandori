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
	err := q.QueryRow(ctx, `
		SELECT target_kcal, target_protein_g, target_carbs_g, target_fat_g,
		       exclusions, allergies,
		       meal_out_kcal, meal_out_protein_g, meal_out_carbs_g, meal_out_fat_g,
		       untracked_kcal, untracked_protein_g, untracked_carbs_g, untracked_fat_g
		FROM profile`).Scan(
		&p.Targets.Kcal, &p.Targets.ProteinG, &p.Targets.CarbsG, &p.Targets.FatG,
		&p.Exclusions, &p.Allergies,
		&p.MealOut.Kcal, &p.MealOut.ProteinG, &p.MealOut.CarbsG, &p.MealOut.FatG,
		&p.UntrackedAllowance.Kcal, &p.UntrackedAllowance.ProteinG, &p.UntrackedAllowance.CarbsG, &p.UntrackedAllowance.FatG,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Profile{}, false, nil
	}
	if err != nil {
		return domain.Profile{}, false, err
	}
	return p, true, nil
}

func saveProfile(ctx context.Context, q db.Querier, p domain.Profile) error {
	_, err := q.Exec(ctx, `
		INSERT INTO profile (
			target_kcal, target_protein_g, target_carbs_g, target_fat_g,
			exclusions, allergies,
			meal_out_kcal, meal_out_protein_g, meal_out_carbs_g, meal_out_fat_g,
			untracked_kcal, untracked_protein_g, untracked_carbs_g, untracked_fat_g
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
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
			updated_at = now()`,
		p.Targets.Kcal, p.Targets.ProteinG, p.Targets.CarbsG, p.Targets.FatG,
		p.Exclusions, p.Allergies,
		p.MealOut.Kcal, p.MealOut.ProteinG, p.MealOut.CarbsG, p.MealOut.FatG,
		p.UntrackedAllowance.Kcal, p.UntrackedAllowance.ProteinG, p.UntrackedAllowance.CarbsG, p.UntrackedAllowance.FatG,
	)
	return err
}
