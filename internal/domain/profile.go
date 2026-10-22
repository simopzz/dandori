package domain

import (
	"slices"
	"strings"
	"unicode"
)

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
	// Exclusions and Allergies are foods the user never eats, as normalised
	// food terms; see Profile.Forbids.
	Exclusions         []string
	Allergies          []string
	MealOut            Nutrition
	UntrackedAllowance Nutrition
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
	return problems
}

func (n Nutrition) amounts() map[string]int {
	return map[string]int{"Kcal": n.Kcal, "ProteinG": n.ProteinG, "CarbsG": n.CarbsG, "FatG": n.FatG}
}

// Forbids reports whether the Ingredient named ingredient contains one of
// the Profile's allergies or exclusions, and which. A term matches when its
// words appear together in the Ingredient's name, ignoring case and plurals:
// "peanuts" forbids "peanut butter", and "pork" forbids "minced pork" but
// not "porky's sauce". Allergies are checked first.
//
// It matches words, not food categories: "shellfish" does not forbid
// "prawns". Recipe generation must still be told the terms themselves.
func (p Profile) Forbids(ingredient string) (term string, forbidden bool) {
	name := foodWords(ingredient)
	for _, terms := range [][]string{p.Allergies, p.Exclusions} {
		for _, term := range terms {
			if containsWords(name, foodWords(term)) {
				return term, true
			}
		}
	}
	return "", false
}

// foodWords splits a food name into lowercase, singular words.
func foodWords(s string) []string {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for i, w := range words {
		words[i] = singular(w)
	}
	return words
}

// singular undoes the regular English plurals: berries, tomatoes, peaches,
// peanuts. Words too short to be plurals, and those in -ss, are kept.
func singular(w string) string {
	switch {
	case len(w) <= 3 || strings.HasSuffix(w, "ss") || !strings.HasSuffix(w, "s"):
		return w
	case strings.HasSuffix(w, "ies"):
		return strings.TrimSuffix(w, "ies") + "y"
	case strings.HasSuffix(w, "oes"), strings.HasSuffix(w, "ches"), strings.HasSuffix(w, "shes"),
		strings.HasSuffix(w, "xes"), strings.HasSuffix(w, "sses"):
		return strings.TrimSuffix(w, "es")
	default:
		return strings.TrimSuffix(w, "s")
	}
}

// containsWords reports whether words holds sub as a contiguous run.
func containsWords(words, sub []string) bool {
	if len(sub) == 0 {
		return false
	}
	for i := 0; i+len(sub) <= len(words); i++ {
		if slices.Equal(words[i:i+len(sub)], sub) {
			return true
		}
	}
	return false
}

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

func normaliseFood(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}
