package domain

import "strings"

// Nutrition is an amount of calories and macros: a day's worth for the
// Targets and the Untracked Allowance, one meal's worth for a Meal Out.
type Nutrition struct {
	Kcal     int
	ProteinG int
	CarbsG   int
	FatG     int
}

// Profile is the user's stored nutrition settings. There is one per app.
type Profile struct {
	Targets Nutrition
	// Exclusions and Allergies are foods the user never eats, as the user
	// named them, normalised by FoodTerms.
	Exclusions         []string
	Allergies          []string
	MealOut            Nutrition
	UntrackedAllowance Nutrition
	// SlotTimes is the time of day of each kind of Slot.
	SlotTimes SlotTimes
}

// Validate returns what is wrong with the Profile, keyed by field path such
// as "Targets.Kcal" or "UntrackedAllowance.ProteinG". A valid Profile
// returns none.
func (p Profile) Validate() map[string]string {
	problems := map[string]string{}
	for group, n := range map[string]Nutrition{"Targets": p.Targets, "MealOut": p.MealOut, "UntrackedAllowance": p.UntrackedAllowance} {
		for nutrient, v := range n.amounts() {
			if v < 0 {
				problems[group+"."+nutrient] = "Can't be negative"
			}
		}
	}
	if p.Targets.Kcal <= 0 {
		problems["Targets.Kcal"] = "Must be more than 0"
	} else if p.UntrackedAllowance.Kcal >= p.Targets.Kcal {
		// Otherwise no Slot could ever be filled.
		problems["UntrackedAllowance.Kcal"] = "Must be less than the daily calorie Target"
	}
	// Slots follow each other in the order of a day.
	for i, k := range SlotKinds[1:] {
		if before := SlotKinds[i]; p.SlotTimes[k] <= p.SlotTimes[before] {
			problems["SlotTimes."+string(k)] = "Must be after " + string(before)
		}
	}
	return problems
}

func (n Nutrition) amounts() map[string]int {
	return map[string]int{"Kcal": n.Kcal, "ProteinG": n.ProteinG, "CarbsG": n.CarbsG, "FatG": n.FatG}
}

// FoodTerm normalises one food term as typed, such as an appliance, so it
// is compared case-insensitively.
func FoodTerm(s string) string { return normaliseFood(s) }

// FoodTerms normalises a list of foods as the user typed them: one per line
// or comma-separated, compared case-insensitively, without blanks or
// duplicates. The order of first appearance is kept.
func FoodTerms(text string) []string {
	terms := []string{}
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == '\n' || r == ';' }) {
		term := normaliseFood(raw)
		if term == "" || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

// typographicApostrophes folds the apostrophes phones type into a straight
// one, so "fiocchi d’avena" is the known "fiocchi d'avena".
var typographicApostrophes = strings.NewReplacer("’", "'", "‘", "'")

func normaliseFood(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(typographicApostrophes.Replace(s))), " ")
}
