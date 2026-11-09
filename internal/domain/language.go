package domain

// Language is the one language a User's Dandori speaks: its pages and every Food Term
// (ADR-0015), as an ISO 639-1 code.
type Language string

const (
	English Language = "en"
	Italian Language = "it"
)

// Languages lists every Language a User can choose.
var Languages = []Language{English, Italian}

// ParseLanguage reads a Language from its code, reporting whether it is one a User
// can choose.
func ParseLanguage(code string) (Language, bool) {
	for _, l := range Languages {
		if string(l) == code {
			return l, true
		}
	}
	return "", false
}

// FoodTerms are the Food Terms a User has written, each in their Language. Each slice
// that brings one (Ingredients, Recipes, the Profile's exclusions) adds its count here
// and to Any.
type FoodTerms struct {
	Ingredients int
	Portions    int
}

// Any reports whether the User has written any Food Term.
func (f FoodTerms) Any() bool { return f.Ingredients > 0 || f.Portions > 0 }

// LanguageCanChange reports whether a User who has written f can still change their
// Language: only until they have written a Food Term, since an Ingredient is identified
// by its name and mixing languages would split one food in two.
func LanguageCanChange(f FoodTerms) bool { return !f.Any() }
