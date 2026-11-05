package domain_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// Two names are one Ingredient when they normalise alike: case, spaces and apostrophes
// don't count, nor do size and grade words, in the User's Language.
func TestIngredientNamesOfOneFoodNormaliseAlike(t *testing.T) {
	tests := []struct {
		name     string
		language domain.Language
		a, b     string
	}{
		{"case and spaces", domain.English, "  Rolled   Oats ", "rolled oats"},
		{"a typographic apostrophe", domain.Italian, "fiocchi d’avena", "fiocchi d'avena"},
		{"a size word", domain.English, "Large egg", "egg"},
		{"a hyphenated size", domain.English, "extra-large egg", "egg"},
		{"a grade", domain.English, "extra virgin olive oil", "olive oil"},
		{"an Italian size word", domain.Italian, "uovo grande", "uovo"},
		{"an Italian grade", domain.Italian, "olio extravergine d'oliva", "olio d'oliva"},
		{"an Italian grade in two words", domain.Italian, "olio extra vergine d'oliva", "olio d'oliva"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := domain.NormaliseIngredientName(tt.language, tt.a), domain.NormaliseIngredientName(tt.language, tt.b)
			if a != b {
				t.Errorf("%q normalises to %q and %q to %q, want them alike", tt.a, a, tt.b, b)
			}
		})
	}
}

// Different foods stay apart, even when they share a word or all but a letter, and so do
// a singular and its plural: no rule tells them from two foods.
func TestIngredientNamesOfDifferentFoodsNormaliseApart(t *testing.T) {
	tests := []struct {
		name     string
		language domain.Language
		a, b     string
	}{
		{"egg white is not egg", domain.English, "egg white", "egg"},
		{"minced chicken is not chicken breast", domain.English, "minced chicken", "chicken breast"},
		{"fresh pasta is not pasta", domain.Italian, "pasta fresca", "pasta"},
		{"pesce is not pesca", domain.Italian, "pesce", "pesca"},
		{"petto di tacchino is not petto di pollo", domain.Italian, "petto di tacchino", "petto di pollo"},
		{"word order counts", domain.Italian, "riso basmati", "basmati riso"},
		{"a plural", domain.English, "eggs", "egg"},
		{"an Italian plural", domain.Italian, "uova", "uovo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := domain.NormaliseIngredientName(tt.language, tt.a), domain.NormaliseIngredientName(tt.language, tt.b)
			if a == b {
				t.Errorf("%q and %q both normalise to %q, want them apart", tt.a, tt.b, a)
			}
		})
	}
}

// A nutritional value is read as typed: blank is unknown, which is never 0, and a value
// is not negative, to one decimal place at most, a comma being a decimal point too.
func TestAnAmountIsReadAsTyped(t *testing.T) {
	tests := []struct {
		typed   string
		want    domain.Amount
		problem domain.Problem
	}{
		{"", domain.Unknown, ""},
		{"  ", domain.Unknown, ""},
		{"0", domain.Known(0), ""},
		{"12", domain.Known(12), ""},
		{"12.5", domain.Known(12.5), ""},
		{"12,5", domain.Known(12.5), ""},
		{" 3.0 ", domain.Known(3), ""},
		{"12.55", domain.Unknown, domain.TooPrecise},
		{"-1", domain.Unknown, domain.Negative},
		{"lots", domain.Unknown, domain.NotANumber},
		{"NaN", domain.Unknown, domain.NotANumber},
		{"10000", domain.Unknown, domain.TooLarge},
	}
	for _, tt := range tests {
		got, problem := domain.ParseAmount(tt.typed)
		if got != tt.want || problem != tt.problem {
			t.Errorf("ParseAmount(%q) = %v, %q; want %v, %q", tt.typed, got, problem, tt.want, tt.problem)
		}
	}
}

// A new Ingredient needs a name, a unit, a usual Storage Location, and its calories,
// protein, carbs and fat; its fibre and sugar may be unknown.
func TestAnIngredientNeedsItsFourNutritionalValues(t *testing.T) {
	egg := domain.Ingredient{
		Name: "egg", Unit: domain.Pieces, Usual: domain.Fridge,
		Nutrition: domain.Nutrition{Kcal: domain.Known(70), Protein: domain.Known(6.3), Carbs: domain.Known(0.4), Fat: domain.Known(4.8)},
	}
	if got := egg.Problems(); len(got) != 0 {
		t.Errorf("an egg with unknown fibre and sugar has problems %v, want none", got)
	}

	var blank domain.Ingredient
	want := map[string]domain.Problem{
		"name": domain.Required, "unit": domain.Required, "usual": domain.Required,
		"kcal": domain.Required, "protein": domain.Required, "carbs": domain.Required, "fat": domain.Required,
	}
	got := blank.Problems()
	if len(got) != len(want) {
		t.Errorf("a blank Ingredient has problems %v, want %v", got, want)
	}
	for field, p := range want {
		if got[field] != p {
			t.Errorf("a blank Ingredient's %s has problem %q, want %q", field, got[field], p)
		}
	}

	odd := egg
	odd.Unit, odd.Usual = "cup", "cellar"
	if got := odd.Problems(); got["unit"] != domain.Required || got["usual"] != domain.Required {
		t.Errorf("an Ingredient in cups kept in the cellar has problems %v, want unit and usual", got)
	}
}

// A name of only size or grade words keeps them: it names something, if oddly.
func TestAnIngredientNameOfOnlySizeWordsKeepsThem(t *testing.T) {
	if got := domain.NormaliseIngredientName(domain.English, "Large"); got == "" {
		t.Error(`"Large" normalises to nothing, want a name`)
	}
	if got := domain.NormaliseIngredientName(domain.English, "   "); got != "" {
		t.Errorf("a blank name normalises to %q, want nothing", got)
	}
}
