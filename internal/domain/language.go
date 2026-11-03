package domain

// Language is the one language a User's Dandori speaks: its pages and
// every food term in their kitchen (ADR-0015), as an ISO 639-1 code.
type Language string

const (
	English Language = "en"
	Italian Language = "it"
)

// Languages lists every Language a User can choose.
var Languages = []Language{English, Italian}

// ParseLanguage reads a Language from its code, reporting whether it is
// one a User can choose.
func ParseLanguage(code string) (Language, bool) {
	for _, l := range Languages {
		if string(l) == code {
			return l, true
		}
	}
	return "", false
}

// Kitchen is what a User's kitchen holds, as far as the rules here need
// it. Each slice that brings a food term (Ingredients, Recipes, the
// Profile's exclusions) adds it here and to holdsFoodTerm.
type Kitchen struct{}

// holdsFoodTerm reports whether the kitchen holds any food term, which is
// written in the User's Language.
func (k Kitchen) holdsFoodTerm() bool { return false }

// LanguageCanChange reports whether a User whose kitchen holds k can still
// change their Language: only until it holds a food term, since an
// Ingredient is identified by its name and mixing languages would split
// one food in two.
func LanguageCanChange(k Kitchen) bool { return !k.holdsFoodTerm() }
