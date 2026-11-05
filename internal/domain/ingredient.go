package domain

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Unit is the one unit an Ingredient is measured in, and its nutritional values are per
// 100 of, or per one piece.
type Unit string

const (
	Grams       Unit = "g"
	Millilitres Unit = "ml"
	Pieces      Unit = "piece"
)

// Units lists every Unit.
var Units = []Unit{Grams, Millilitres, Pieces}

// StorageLocation is where a Portion or Stock Item is kept.
type StorageLocation string

const (
	Pantry  StorageLocation = "pantry"
	Fridge  StorageLocation = "fridge"
	Freezer StorageLocation = "freezer"
)

// StorageLocations lists every StorageLocation.
var StorageLocations = []StorageLocation{Pantry, Fridge, Freezer}

// Ingredient is a food as such, as the User added it.
type Ingredient struct {
	// Name is as the User typed it; NormaliseIngredientName makes the key that
	// identifies it.
	Name string
	Unit Unit
	// Nutrition is per 100 g, per 100 ml or per piece, as Unit says.
	Nutrition Nutrition
	// Usual is the Storage Location it is usually kept in.
	Usual  StorageLocation
	Staple bool
}

// Nutrition is an Ingredient's nutritional values: calories, and grams of the rest.
// Fibre and sugar may be Unknown; the others an Ingredient always has.
type Nutrition struct {
	Kcal, Protein, Carbs, Fat, Fibre, Sugar Amount
}

// Amount is a nutritional value, or Unknown, which is never 0.
type Amount struct {
	Value float64
	Known bool
}

// Unknown is a nutritional value the User didn't give.
var Unknown = Amount{}

// Known is the nutritional value v.
func Known(v float64) Amount { return Amount{Value: v, Known: true} }

// Problem is why a value is refused.
type Problem string

const (
	Required   Problem = "required"
	NotANumber Problem = "not a number"
	Negative   Problem = "negative"
	TooPrecise Problem = "more than one decimal place"
	TooLarge   Problem = "too large"
)

// maxAmount bounds a nutritional value: no food holds 10000 kcal in 100 g or a piece.
const maxAmount = 10000

// ParseAmount reads a nutritional value as the User typed it: blank is Unknown, and a
// value is a number not negative, to one decimal place at most, with a point or a comma.
func ParseAmount(typed string) (Amount, Problem) {
	typed = strings.ReplaceAll(strings.TrimSpace(typed), ",", ".")
	if typed == "" {
		return Unknown, ""
	}
	v, err := strconv.ParseFloat(typed, 64)
	switch {
	case err != nil || math.IsNaN(v) || math.IsInf(v, 0):
		return Unknown, NotANumber
	case v < 0:
		return Unknown, Negative
	case v >= maxAmount:
		return Unknown, TooLarge
	case math.Abs(v*10-math.Round(v*10)) > 1e-9:
		return Unknown, TooPrecise
	}
	return Known(v), ""
}

// Problems is what is missing from i, by field: its name, unit, usual Storage Location,
// and the four nutritional values every Ingredient has. A whole Ingredient has none.
func (i Ingredient) Problems() map[string]Problem {
	problems := map[string]Problem{}
	if strings.TrimSpace(i.Name) == "" {
		problems["name"] = Required
	}
	if !slices.Contains(Units, i.Unit) {
		problems["unit"] = Required
	}
	if !slices.Contains(StorageLocations, i.Usual) {
		problems["usual"] = Required
	}
	for field, a := range map[string]Amount{"kcal": i.Nutrition.Kcal, "protein": i.Nutrition.Protein, "carbs": i.Nutrition.Carbs, "fat": i.Nutrition.Fat} {
		if !a.Known {
			problems[field] = Required
		}
	}
	return problems
}

// NormaliseIngredientName is the key that identifies an Ingredient named name by a User
// speaking l: two names with one key are one food, and a User's Ingredients each have
// their own. Case, spaces and typographic apostrophes don't count, as in v1, nor do size
// and grade words ("large eggs", "extra virgin olive oil"), and each word is reduced to a
// stem its singular and plural share. The key is never shown, so it needn't be a word:
// "uova" and "uovo" are both "uov".
func NormaliseIngredientName(l Language, name string) string {
	words := strings.Fields(strings.ToLower(typographicApostrophes.Replace(name)))
	if kept := withoutSizeWords(l, words); len(kept) > 0 {
		words = kept
	}
	stem := stems[l]
	for i, w := range words {
		if len([]rune(w)) > 2 {
			words[i] = stem(w)
		}
	}
	return strings.Join(words, " ")
}

// typographicApostrophes folds the apostrophes phones type into a straight one, so
// "fiocchi d’avena" is "fiocchi d'avena".
var typographicApostrophes = strings.NewReplacer("’", "'", "‘", "'")

// sizeWords are the size and grade words of each Language, which don't make a food
// another Ingredient: one word, or a run of them.
var sizeWords = map[Language][][]string{
	English: {
		{"extra", "virgin"}, {"extra", "large"}, {"extra-virgin"}, {"extra-large"},
		{"small"}, {"medium"}, {"large"}, {"jumbo"},
	},
	Italian: {
		{"extra", "vergine"}, {"extravergine"},
		{"piccolo"}, {"piccola"}, {"piccoli"}, {"piccole"},
		{"medio"}, {"media"}, {"medi"}, {"medie"},
		{"grande"}, {"grandi"},
	},
}

// withoutSizeWords is words with l's size and grade words left out.
func withoutSizeWords(l Language, words []string) []string {
	var kept []string
next:
	for i := 0; i < len(words); i++ {
		for _, run := range sizeWords[l] {
			if i+len(run) <= len(words) && slices.Equal(words[i:i+len(run)], run) {
				i += len(run) - 1
				continue next
			}
		}
		kept = append(kept, words[i])
	}
	return kept
}

// stems reduce a word of each Language to what its singular and plural share.
var stems = map[Language]func(string) string{
	English: englishStem,
	Italian: italianStem,
}

// englishStem drops a plural's -s, or turns -ies into -y, then drops a final e, so
// "cheeses" and "cheese" are both "chees", and "tomatoes" and "tomato" "tomato".
func englishStem(w string) string {
	switch {
	case strings.HasSuffix(w, "ies"):
		w = strings.TrimSuffix(w, "ies") + "y"
	case strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss"):
		w = strings.TrimSuffix(w, "s")
	}
	return strings.TrimSuffix(w, "e")
}

// italianStem drops the final vowel that tells singular from plural, then the h that
// keeps a -chi or -ghi plural hard and the i of a -cia or -io singular, so "funghi" and
// "fungo" are both "fung", and "arance" and "arancia" "aranc".
func italianStem(w string) string {
	if strings.ContainsAny(w[len(w)-1:], "aeio") {
		w = w[:len(w)-1]
	}
	if strings.HasSuffix(w, "ch") || strings.HasSuffix(w, "gh") {
		w = w[:len(w)-1]
	}
	return strings.TrimSuffix(w, "i")
}
