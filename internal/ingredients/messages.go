package ingredients

import (
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// The Ingredients' Messages.
var (
	msgIngredients      = web.Message{En: "Ingredients", It: "Ingredienti"}
	msgIngredientsTitle = web.Message{En: "Ingredients · Dandori", It: "Ingredienti · Dandori"}
	msgAddIngredient    = web.Message{En: "Add an Ingredient", It: "Aggiungi un Ingrediente"}
	msgYourIngredients  = web.Message{En: "Your Ingredients", It: "I tuoi Ingredienti"}
	msgNoIngredients    = web.Message{En: "No Ingredients yet.", It: "Ancora nessun Ingrediente."}
	msgAdded            = web.Message{En: "Added %s.", It: "Aggiunto: %s."}
	msgAdd              = web.Message{En: "Add", It: "Aggiungi"}
	msgName             = web.Message{En: "Name", It: "Nome"}
	msgUnit             = web.Message{En: "Unit", It: "Unità"}
	msgUsual            = web.Message{En: "Usually kept in", It: "Di solito in"}
	msgChoose           = web.Message{En: "Choose…", It: "Scegli…"}
	msgNutritionHint    = web.Message{
		En: "Per 100 g, per 100 ml or per piece, to match the unit, as on the label: calories in kcal, the rest in grams. Fibre and sugar may be left blank.",
		It: "Per 100 g, per 100 ml o per pezzo, secondo l'unità, come in etichetta: calorie in kcal, il resto in grammi. Fibre e zuccheri si possono lasciare vuoti.",
	}
	msgKcal         = web.Message{En: "Calories", It: "Calorie"}
	msgProtein      = web.Message{En: "Protein", It: "Proteine"}
	msgCarbs        = web.Message{En: "Carbs", It: "Carboidrati"}
	msgFat          = web.Message{En: "Fat", It: "Grassi"}
	msgFibre        = web.Message{En: "Fibre", It: "Fibre"}
	msgSugar        = web.Message{En: "Sugar", It: "Zuccheri"}
	msgStaple       = web.Message{En: "Staple", It: "Base"}
	msgStapleHint   = web.Message{En: "always there, never counted", It: "sempre in casa, mai contata"}
	msgKcalValue    = web.Message{En: "%s kcal", It: "%s kcal"}
	msgProteinShort = web.Message{En: "protein", It: "proteine"}
	msgCarbsShort   = web.Message{En: "carbs", It: "carboidrati"}
	msgFatShort     = web.Message{En: "fat", It: "grassi"}
	msgFibreShort   = web.Message{En: "fibre", It: "fibre"}
	msgSugarShort   = web.Message{En: "sugar", It: "zuccheri"}
	msgGrams        = web.Message{En: "%s g", It: "%s g"}
	msgUnknown      = web.Message{En: "unknown", It: "non noto"}

	msgNameRequired  = web.Message{En: "Give it a name.", It: "Dagli un nome."}
	msgUnitRequired  = web.Message{En: "Choose g, ml or piece.", It: "Scegli g, ml o pezzo."}
	msgUsualRequired = web.Message{En: "Choose where it is usually kept.", It: "Scegli dove si tiene di solito."}
	msgDidYouMean    = web.Message{En: "Did you mean %s?", It: "Intendevi %s?"}
	msgAddAnyway     = web.Message{En: "Add %s anyway", It: "Aggiungi %s comunque"}
	msgPick          = web.Message{En: "Use %s", It: "Usa %s"}
	msgPicked        = web.Message{En: "%s is already one of your Ingredients.", It: "%s è già tra i tuoi Ingredienti."}
	msgSearch        = web.Message{En: "Search", It: "Cerca"}
	msgNoMatch       = web.Message{En: "No Ingredient is like %s.", It: "Nessun Ingrediente somiglia a %s."}
	msgNameTaken     = web.Message{
		En: "You already have %s: one food is one Ingredient.",
		It: "Hai già %s: un cibo è un solo Ingrediente.",
	}
)

// problemMessages say why a nutritional value is refused.
var problemMessages = map[domain.Problem]web.Message{
	domain.Required:   {En: "Required.", It: "Obbligatorio."},
	domain.NotANumber: {En: "Type a number.", It: "Scrivi un numero."},
	domain.Negative:   {En: "Can't be below 0.", It: "Non può essere sotto 0."},
	domain.TooPrecise: {En: "One decimal place at most.", It: "Al massimo un decimale."},
	domain.TooLarge:   {En: "Must be below 10000.", It: "Deve essere sotto 10000."},
}

// unitNames name each Unit as a choice; perUnit is what nutritional values are per.
var (
	unitNames = map[domain.Unit]web.Message{
		domain.Grams:       {En: "g", It: "g"},
		domain.Millilitres: {En: "ml", It: "ml"},
		domain.Pieces:      {En: "piece", It: "pezzo"},
	}
	perUnit = map[domain.Unit]web.Message{
		domain.Grams:       {En: "per 100 g", It: "per 100 g"},
		domain.Millilitres: {En: "per 100 ml", It: "per 100 ml"},
		domain.Pieces:      {En: "per piece", It: "per pezzo"},
	}
)

// storageLocationNames name each Storage Location.
var storageLocationNames = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "pantry", It: "dispensa"},
	domain.Fridge:  {En: "fridge", It: "frigo"},
	domain.Freezer: {En: "freezer", It: "congelatore"},
}
