package domain_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// kitchenNames are real food names to stress the matcher with: Pak'nSave's pantry staples
// and goblinbox.com's list in English; Melarossa's and Grana Padano's shopping lists and
// Wikipedia's Ortaggio in Italian, with common meat, cheese and cured meat names added.
var kitchenNames = map[domain.Language][]string{
	domain.English: {
		"almonds", "apple cider vinegar", "apples", "apricots", "arborio rice", "avocados",
		"bacon", "baked beans", "baking powder", "baking soda", "balsamic vinegar",
		"bananas", "barbecue sauce", "basil", "basmati rice", "bay leaf", "beef mince",
		"beef stock powder", "beetroot", "bell peppers", "black beans", "black pepper",
		"bread", "breadcrumbs", "broccoli", "brown rice", "brown sugar", "bulgar wheat",
		"butter", "cannellini beans", "canola oil", "capers", "carrots", "cashew nuts",
		"caster sugar", "cauliflower", "cayenne pepper", "celery", "cheese",
		"chicken breast", "chicken stock powder", "chickpeas", "chilli flakes",
		"chilli powder", "chocolate", "cinnamon", "cocoa powder", "coconut milk", "coffee",
		"condensed milk", "corn", "corned beef", "cornflour", "cornmeal", "courgette",
		"couscous", "crackers", "cumin", "curry paste", "curry powder", "dates",
		"egg noodles", "eggs", "evaporated milk", "fish sauce", "flour", "frozen peas",
		"garam masala", "garlic", "garlic powder", "gelatin", "ginger", "green beans",
		"ground coriander", "ground ginger", "himalayan salt", "honey", "hot sauce",
		"hummus", "ice cream", "icing sugar", "jam", "jasmine rice", "kale", "kidney beans",
		"kosher salt", "lemons", "lentils", "lettuce", "lime", "macaroni", "maple syrup",
		"mayonnaise", "milk", "miso", "mushrooms", "mustard", "noodles", "nutmeg",
		"olive oil", "olives", "onions", "oregano", "parmesan", "passata", "pasta",
		"peaches", "peanut butter", "peanut oil", "peanuts", "pears", "peppercorns",
		"pine nuts", "pineapple", "plain flour", "plain yoghurt", "polenta", "popcorn",
		"potatoes", "prawns", "pretzels", "puff pastry", "quinoa", "raisins", "raspberry",
		"red wine vinegar", "rice", "rice bran oil", "rice vinegar", "rolled oats",
		"rosemary", "salmon", "salt", "sardines", "sausages", "sesame oil",
		"sherry vinegar", "smoked paprika", "sour cream", "soy sauce", "spaghetti",
		"spinach", "spring onions", "strawberry", "tahini", "tea", "thyme", "tomato paste",
		"tomato sauce", "tomatoes", "tuna", "vanilla extract", "vegetable oil",
		"vegetable stock powder", "walnuts", "white sugar", "white vinegar",
		"white wine vinegar", "yeast",
	},
	domain.Italian: {
		"aceto", "aceto balsamico", "aglio", "agnello", "arance", "asparagi", "avena",
		"baccalà", "barbabietola", "basilico", "bietole", "biscotti secchi", "borragine",
		"bresaola", "broccoli", "burro", "burro di arachidi", "caffè", "capperi",
		"carciofi", "cardo", "carne", "carote", "catalogna", "cavolfiore", "cavolo", "cece",
		"ceci", "cereali", "cervo", "cetriolo", "cicoria", "cinghiale",
		"cioccolato fondente", "cipolla", "cipollotto", "clementine", "coniglio",
		"coscia di pollo", "cous cous", "crescione", "curcuma", "fagioli", "fagiolini",
		"faraona", "farfalle integrali", "farina", "farina di mais", "farina di riso",
		"fava", "fette biscottate", "finocchi", "fiocchi d'avena", "fragole", "funghi",
		"fungo", "gallette di grano saraceno", "gallette di mais", "gallette di riso",
		"gnocchi di patate", "grana padano", "grissini torinesi", "hamburger di manzo",
		"hummus", "indivia", "kefir", "kiwi", "latte", "latte di cocco", "latte di soia",
		"lattuga", "lenticchie", "lepre", "lievito", "limone", "maiale", "mais", "mandorle",
		"manzo", "marmellata", "mascarpone", "mela", "melanzane", "mele", "merluzzo",
		"miele", "mozzarella", "nocciola", "nocciole", "noce", "noci", "olio di arachidi",
		"olio di semi", "olio extravergine d'oliva", "orecchiette", "orzo", "pancetta",
		"pane", "pane integrale", "pangrattato", "panna", "parmigiano", "passata", "pasta",
		"pastinaca", "patate", "pecorino", "pepe", "peperoncino", "peperone", "pera",
		"pere", "pesca", "pesce", "pesce spada", "pesto", "petto di pollo",
		"petto di tacchino", "piadine", "piselli", "pollo", "pomodori pelati", "pomodoro",
		"porro", "prezzemolo", "prosciutto cotto", "prosciutto crudo", "radicchio", "rapa",
		"ravanello", "ricotta", "riso", "rosmarino", "rucola", "salame", "sale", "salmone",
		"salvia", "scalogno", "sedano", "sgombro", "songino", "speck", "spinaci",
		"tacchino", "tempeh", "timo", "tofu", "tonno", "topinambur", "tè", "uova", "vino",
		"vitello", "yogurt", "zucca", "zucchero di canna", "zucchine",
	},
}

// typosOf are name's single-letter slips: each letter dropped, and each pair swapped.
func typosOf(l domain.Language, name string) []string {
	r := []rune(name)
	var typos []string
	add := func(typo string) {
		if domain.NormaliseIngredientName(l, typo) != domain.NormaliseIngredientName(l, name) {
			typos = append(typos, typo)
		}
	}
	for i := range r {
		if r[i] != ' ' && r[i] != '\'' {
			add(string(r[:i]) + string(r[i+1:]))
		}
		if i+1 < len(r) && r[i] != ' ' && r[i+1] != ' ' && r[i] != r[i+1] {
			swapped := slices.Clone(r)
			swapped[i], swapped[i+1] = r[i+1], r[i]
			add(string(swapped))
		}
	}
	return typos
}

// without is names with name left out.
func without(names []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(names), func(n string) bool { return n == name })
}

// A name typed exactly comes first in a search, ahead of the longer names it is a word
// of: milk before coconut milk, however the names sort.
func TestSearchingRanksTheExactNameFirst(t *testing.T) {
	for l, names := range kitchenNames {
		for _, name := range names {
			if found := domain.SearchIngredients(name, names); len(found) == 0 || names[found[0]] != name {
				t.Errorf("%s: SearchIngredients(%q) doesn't rank it first: %v", l, name, found)
			}
		}
	}
}

// Every word of a name finds it, so a User remembering part of a name reaches it.
func TestSearchingFindsANameByAnyOfItsWords(t *testing.T) {
	for l, names := range kitchenNames {
		for i, name := range names {
			for _, word := range strings.Fields(name) {
				if !slices.Contains(domain.SearchIngredients(word, names), i) {
					t.Errorf("%s: searching for %q doesn't find %q", l, word, name)
				}
			}
		}
	}
}

// Real slips suggest the name meant, among every name of the Language: letters doubled
// or dropped, and the accents a phone leaves off.
func TestClosestIngredientFindsRealSlips(t *testing.T) {
	tests := []struct {
		language    domain.Language
		typed, want string
	}{
		{domain.English, "brocccoli", "broccoli"},
		{domain.English, "cinammon", "cinnamon"},
		{domain.English, "mayonaise", "mayonnaise"},
		{domain.English, "parmesean", "parmesan"},
		{domain.English, "coliflower", "cauliflower"},
		{domain.English, "chick peas", "chickpeas"},
		{domain.Italian, "mozarella", "mozzarella"},
		{domain.Italian, "parmiggiano", "parmigiano"},
		{domain.Italian, "prezzemmolo", "prezzemolo"},
		{domain.Italian, "melanzana", "melanzane"},
		{domain.Italian, "caffe", "caffè"},
		{domain.Italian, "baccala", "baccalà"},
	}
	for _, tt := range tests {
		t.Run(tt.typed, func(t *testing.T) {
			got, ok := domain.ClosestIngredient(tt.language, tt.typed, kitchenNames[tt.language])
			if !ok || got != tt.want {
				t.Errorf("ClosestIngredient(%q) = %q, %v; want %q", tt.typed, got, ok, tt.want)
			}
		})
	}
}

// What the matcher gets wrong, kept in sight. Trigrams reward shared words and shared
// starts, so it suggests another food, which the User ignores, and a slip in a short word
// breaks too many of its few trigrams to suggest anything, so a duplicate gets through.
func TestClosestIngredientsKnownLimits(t *testing.T) {
	suggestsAnotherFood := []struct {
		language    domain.Language
		typed, want string
	}{
		{domain.English, "peanut butter", "butter"},
		{domain.English, "garlic powder", "garlic"},
		{domain.English, "chicken stock powder", "beef stock powder"},
		{domain.Italian, "pesce", "pesca"},
		{domain.Italian, "peperoncino", "peperone"},
		{domain.Italian, "prosciutto cotto", "prosciutto crudo"},
		{domain.Italian, "gallette di riso", "gallette di mais"},
	}
	for _, tt := range suggestsAnotherFood {
		got, ok := domain.ClosestIngredient(tt.language, tt.typed, without(kitchenNames[tt.language], tt.typed))
		if !ok || got != tt.want {
			t.Errorf("ClosestIngredient(%q) = %q, %v; want %q", tt.typed, got, ok, tt.want)
		}
	}
	misses := []struct {
		language domain.Language
		typed    string
	}{
		{domain.English, "slt"}, {domain.English, "bsail"}, {domain.English, "hyme"},
		{domain.Italian, "aglo"}, {domain.Italian, "ltte"}, {domain.Italian, "limne"}, {domain.Italian, "cuscus"},
	}
	for _, tt := range misses {
		if got, ok := domain.ClosestIngredient(tt.language, tt.typed, kitchenNames[tt.language]); ok {
			t.Errorf("ClosestIngredient(%q) = %q; this limit is gone, move it to the slips it finds", tt.typed, got)
		}
	}
}

// How often a slip of each name finds it, across every slip of every name. These floors
// are what the matcher does today, so a change can only raise them: raise them with it.
func TestSlipsOfKitchenNames(t *testing.T) {
	band := func(name string) string {
		switch n := len([]rune(name)); {
		case n <= 5:
			return "up to 5 letters"
		case n <= 8:
			return "6 to 8 letters"
		}
		return "9 letters or more"
	}
	floors := map[string]float64{
		"en up to 5 letters": 0.12, "en 6 to 8 letters": 0.46, "en 9 letters or more": 0.91, "en search": 0.93,
		"it up to 5 letters": 0.15, "it 6 to 8 letters": 0.47, "it 9 letters or more": 0.86, "it search": 0.89,
	}
	for l, names := range kitchenNames {
		total, suggested, searched := map[string]int{}, map[string]int{}, 0
		slips := 0
		for _, name := range names {
			for _, typo := range typosOf(l, name) {
				b := band(name)
				total[b]++
				slips++
				if got, ok := domain.ClosestIngredient(l, typo, names); ok && got == name {
					suggested[b]++
				}
				found := domain.SearchIngredients(typo, names)
				if at := slices.IndexFunc(found, func(i int) bool { return names[i] == name }); at >= 0 && at < 3 {
					searched++
				}
			}
		}
		for b, n := range total {
			rate := float64(suggested[b]) / float64(n)
			t.Logf("%s, %s: %.3f of %d slips suggest the name", l, b, rate, n)
			if floor := floors[fmt.Sprintf("%s %s", l, b)]; rate < floor {
				t.Errorf("%s, %s: %.3f of slips suggest the name, want at least %.3f", l, b, rate, floor)
			}
		}
		rate := float64(searched) / float64(slips)
		t.Logf("%s: %.3f of %d slips find the name in a search's first three", l, rate, slips)
		if floor := floors[fmt.Sprintf("%s search", l)]; rate < floor {
			t.Errorf("%s: %.3f of slips find the name in a search's first three, want at least %.3f", l, rate, floor)
		}
	}
}

// Most real names suggest no other: an Ingredient of a new food usually goes straight in.
func TestMostKitchenNamesSuggestNoOther(t *testing.T) {
	const ceiling = 0.13
	for l, names := range kitchenNames {
		suggesting := 0
		for _, name := range names {
			if _, ok := domain.ClosestIngredient(l, name, without(names, name)); ok {
				suggesting++
			}
		}
		if rate := float64(suggesting) / float64(len(names)); rate > ceiling {
			t.Errorf("%s: %d of %d names suggest another, more than %.2f", l, suggesting, len(names), ceiling)
		}
	}
}
