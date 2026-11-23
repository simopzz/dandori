package domain

import (
	"slices"
	"strings"
)

// NormaliseFoods reads a `typed` list of foods, the Profile's exclusions or allergies:
// one per line or separated by commas or semicolons, each in lower case with its inner
// space collapsed and typographic apostrophes folded, blanks and repeats dropped, in the
// order they first appear. It is never nil, so an empty list is saved as empty.
func NormaliseFoods(typed string) []string {
	foods := []string{}
	for _, raw := range strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == ';' || r == '\n' }) {
		food := strings.Join(strings.Fields(strings.ToLower(typographicApostrophes.Replace(raw))), " ")
		if food != "" && !slices.Contains(foods, food) {
			foods = append(foods, food)
		}
	}
	return foods
}
