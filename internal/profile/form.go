package profile

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/simopzz/dandori/internal/domain"
)

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
	p.SlotTimes = domain.SlotTimes{}
	for _, k := range domain.SlotKinds {
		name := slotTimeField(k)
		switch t, err := domain.ParseTimeOfDay(get(name)); {
		case get(name) == "":
			problems[name] = "Required"
		case err != nil:
			problems[name] = "Must be a time"
		default:
			p.SlotTimes[k] = t
		}
	}

	invalid := p.Validate()
	for _, f := range numberFields {
		if msg, ok := invalid[f.path]; ok && problems[f.name] == "" {
			problems[f.name] = msg
		}
	}
	for _, k := range domain.SlotKinds {
		if msg, ok := invalid["SlotTimes."+string(k)]; ok && problems[slotTimeField(k)] == "" {
			problems[slotTimeField(k)] = msg
		}
	}
	return p, problems
}

// slotTimeField is the form field of the time of day of Slots of kind k,
// e.g. "lunch_at".
func slotTimeField(k domain.SlotKind) string { return string(k) + "_at" }

func profileValues(p domain.Profile) map[string]string {
	values := map[string]string{}
	for _, f := range numberFields {
		values[f.name] = strconv.Itoa(*f.of(&p))
	}
	values["exclusions"] = strings.Join(p.Exclusions, "\n")
	values["allergies"] = strings.Join(p.Allergies, "\n")
	for _, k := range domain.SlotKinds {
		values[slotTimeField(k)] = p.SlotTimes[k].String()
	}
	return values
}
