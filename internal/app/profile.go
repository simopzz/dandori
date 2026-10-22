package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"github.com/simopzz/mealops/internal/domain"
	"github.com/simopzz/mealops/internal/web"
)

func (a *App) showProfile(w http.ResponseWriter, r *http.Request) {
	p, found, err := a.loadProfile(r.Context())
	if err != nil {
		a.serverError(w, "load profile", err)
		return
	}
	f := web.ProfileForm{Values: map[string]string{}, Saved: r.URL.Query().Has("saved")}
	if found {
		f.Values = profileValues(p)
	}
	templ.Handler(web.ProfilePage(f)).ServeHTTP(w, r)
}

func (a *App) saveProfile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	typed := map[string]string{}
	for name := range r.PostForm {
		typed[name] = r.PostForm.Get(name)
	}
	p, problems := parseProfile(r.PostForm)
	if len(problems) > 0 {
		a.renderProfileForm(w, r, web.ProfileForm{Values: typed, Errors: problems}, http.StatusUnprocessableEntity)
		return
	}
	if err := a.storeProfile(r.Context(), p); err != nil {
		// Keep what was typed rather than swapping a bare error over the form.
		a.log.Error("save profile", "err", err)
		a.renderProfileForm(w, r, web.ProfileForm{Values: typed, Failed: true}, http.StatusInternalServerError)
		return
	}
	if !isHTMX(r) {
		// Redirect, so reloading the page doesn't resubmit the form.
		http.Redirect(w, r, "/profile?saved", http.StatusSeeOther)
		return
	}
	a.renderProfileForm(w, r, web.ProfileForm{Values: profileValues(p), Saved: true}, http.StatusOK)
}

// renderProfileForm answers a submission: HTMX swaps the form in place,
// a plain browser gets the whole page.
func (a *App) renderProfileForm(w http.ResponseWriter, r *http.Request, f web.ProfileForm, status int) {
	c := web.ProfilePage(f)
	if isHTMX(r) {
		c = web.ProfileFormFragment(f)
	}
	templ.Handler(c, templ.WithStatus(status)).ServeHTTP(w, r)
}

// numberField is one numeric form field and the Profile amount it fills.
type numberField struct {
	name string // form field name, e.g. "target_kcal"
	path string // domain field path, e.g. "Targets.Kcal"
	of   func(*domain.Profile) *int
}

// numberFields lists every numeric field of the Profile form.
var numberFields = func() []numberField {
	groups := []struct {
		prefix, path string
		of           func(*domain.Profile) *domain.Nutrition
	}{
		{"target", "Targets", func(p *domain.Profile) *domain.Nutrition { return &p.Targets }},
		{"meal_out", "MealOut", func(p *domain.Profile) *domain.Nutrition { return &p.MealOut }},
		{"untracked", "UntrackedAllowance", func(p *domain.Profile) *domain.Nutrition { return &p.UntrackedAllowance }},
	}
	nutrients := []struct {
		suffix, path string
		of           func(*domain.Nutrition) *int
	}{
		{"_kcal", "Kcal", func(n *domain.Nutrition) *int { return &n.Kcal }},
		{"_protein_g", "ProteinG", func(n *domain.Nutrition) *int { return &n.ProteinG }},
		{"_carbs_g", "CarbsG", func(n *domain.Nutrition) *int { return &n.CarbsG }},
		{"_fat_g", "FatG", func(n *domain.Nutrition) *int { return &n.FatG }},
	}
	var fields []numberField
	for _, g := range groups {
		for _, n := range nutrients {
			fields = append(fields, numberField{
				name: g.prefix + n.suffix,
				path: g.path + "." + n.path,
				of:   func(p *domain.Profile) *int { return n.of(g.of(p)) },
			})
		}
	}
	return fields
}()

// parseProfile reads the Profile form, returning what is wrong with it keyed
// by form field name.
func parseProfile(form url.Values) (domain.Profile, map[string]string) {
	get := func(name string) string { return strings.TrimSpace(form.Get(name)) }
	var p domain.Profile
	problems := map[string]string{}
	for _, f := range numberFields {
		raw := get(f.name)
		if raw == "" {
			problems[f.name] = "Required"
			continue
		}
		v, err := strconv.ParseInt(raw, 10, 32)
		switch {
		case errors.Is(err, strconv.ErrRange):
			problems[f.name] = "Too large"
			continue
		case err != nil:
			problems[f.name] = "Must be a whole number"
			continue
		}
		*f.of(&p) = int(v)
	}
	p.Exclusions = domain.FoodTerms(get("exclusions"))
	p.Allergies = domain.FoodTerms(get("allergies"))

	invalid := p.Validate()
	for _, f := range numberFields {
		if msg, ok := invalid[f.path]; ok && problems[f.name] == "" {
			problems[f.name] = msg
		}
	}
	return p, problems
}

func profileValues(p domain.Profile) map[string]string {
	values := map[string]string{}
	for _, f := range numberFields {
		values[f.name] = strconv.Itoa(*f.of(&p))
	}
	values["exclusions"] = strings.Join(p.Exclusions, "\n")
	values["allergies"] = strings.Join(p.Allergies, "\n")
	return values
}

func (a *App) loadProfile(ctx context.Context) (domain.Profile, bool, error) {
	var p domain.Profile
	err := a.pool.QueryRow(ctx, `
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

func (a *App) storeProfile(ctx context.Context, p domain.Profile) error {
	_, err := a.pool.Exec(ctx, `
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
