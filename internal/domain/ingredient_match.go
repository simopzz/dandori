package domain

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

// suggestionSimilarity is how alike a name must be to a known Ingredient's to suggest it:
// a letter or two off, or the words in another order, but not another food that shares a
// word ("petto di tacchino" is not "petto di pollo").
const suggestionSimilarity = 0.5

// searchSimilarity is how alike a name, or a run of its words, must be to what was typed
// for a search to find it: pg_trgm's default, looser than a suggestion, so a typo in one
// word finds the name it is in.
const searchSimilarity = 0.3

// ClosestIngredient is the known Ingredient name most like name, the one a mistyped name
// most likely meant, if any is alike enough to suggest. Both are compared as
// NormaliseIngredientName reduces them in l, so a size word counts for nothing. Likeness
// is trigram similarity as Postgres's pg_trgm computes it; ties go to the name first
// alphabetically.
func ClosestIngredient(l Language, name string, known []string) (string, bool) {
	typed := trigrams(NormaliseIngredientName(l, name))
	var best string
	bestScore := 0.0
	for _, k := range known {
		score := similarity(typed, trigrams(NormaliseIngredientName(l, k)))
		if score > bestScore || score == bestScore && score > 0 && k < best {
			best, bestScore = k, score
		}
	}
	if bestScore < suggestionSimilarity {
		return "", false
	}
	return best, true
}

// SearchIngredients is which of names a search for query finds, as indices into names,
// most alike first and alike ones in the order given: those that contain query, and those
// whose spelling is alike, whole or in a run of their words, so a typo still finds the
// Ingredient. A blank query finds every name.
func SearchIngredients(query string, names []string) []int {
	query = fold(query)
	found := []int{}
	scores := map[int]float64{}
	for i, name := range names {
		if query == "" {
			found = append(found, i)
			continue
		}
		score := runSimilarity(query, name)
		if strings.Contains(fold(name), query) || score >= searchSimilarity {
			found = append(found, i)
			scores[i] = score
		}
	}
	slices.SortStableFunc(found, func(a, b int) int { return cmp.Compare(scores[b], scores[a]) })
	if len(found) == 0 {
		return nil
	}
	return found
}

// fold is s as a search compares it: trimmed, in lower case, its apostrophes straight.
func fold(s string) string {
	return strings.ToLower(strings.TrimSpace(typographicApostrophes.Replace(s)))
}

// runSimilarity is how alike query is to name, or to the run of name's words most like
// it, whichever is more.
func runSimilarity(query, name string) float64 {
	typed := trigrams(query)
	best := 0.0
	ws := words(name)
	for i := range ws {
		for j := i + 1; j <= len(ws); j++ {
			best = max(best, similarity(typed, trigrams(strings.Join(ws[i:j], " "))))
		}
	}
	return best
}

// words are s's words in lower case, apart at anything but a letter or a digit.
func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

// trigrams is the set of three-letter runs of s's words, each word padded with two spaces
// before and one after, as pg_trgm takes them.
func trigrams(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range words(s) {
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
