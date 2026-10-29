package domain

import (
	"strings"
	"unicode"
)

// suggestionSimilarity is how alike a name must be to a known Ingredient's
// to suggest it: a letter or two off, or the words in another order, but
// not another food that shares a word ("petto di tacchino" is not "petto di
// pollo").
const suggestionSimilarity = 0.5

// ClosestIngredient is the known Ingredient name most like name, the one a
// mistyped name most likely meant, if any is alike enough to suggest.
// Likeness is trigram similarity as Postgres's pg_trgm computes it; ties go
// to the name first alphabetically.
func ClosestIngredient(name string, known []string) (string, bool) {
	typed := trigrams(name)
	var best string
	bestScore := 0.0
	for _, k := range known {
		score := similarity(typed, trigrams(k))
		if score > bestScore || score == bestScore && score > 0 && k < best {
			best, bestScore = k, score
		}
	}
	if bestScore < suggestionSimilarity {
		return "", false
	}
	return best, true
}

// trigrams is the set of three-letter runs of s's words, each word padded
// with two spaces before and one after, as pg_trgm takes them.
func trigrams(s string) map[string]bool {
	set := map[string]bool{}
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		padded := []rune("  " + w + " ")
		for i := 0; i+3 <= len(padded); i++ {
			set[string(padded[i:i+3])] = true
		}
	}
	return set
}

// similarity is the share of a's and b's trigrams that both have.
func similarity(a, b map[string]bool) float64 {
	shared := 0
	for t := range a {
		if b[t] {
			shared++
		}
	}
	all := len(a) + len(b) - shared
	if all == 0 {
		return 0
	}
	return float64(shared) / float64(all)
}
