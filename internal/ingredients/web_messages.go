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
	msgPick          = web.Message{En: "Use %s", It: "Usa %s"}
	msgPicked        = web.Message{En: "%s is already one of your Ingredients.", It: "%s è già tra i tuoi Ingredienti."}
	msgSearch        = web.Message{En: "Search", It: "Cerca"}
	msgNoMatch       = web.Message{En: "No Ingredient is like %s.", It: "Nessun Ingrediente somiglia a %s."}
	msgAddNamed      = web.Message{En: "Add %s as a new Ingredient", It: "Aggiungi %s come nuovo Ingrediente"}
	msgSave          = web.Message{En: "Save", It: "Salva"}
	msgSaved         = web.Message{En: "Saved %s.", It: "Salvato: %s."}
	msgDelete        = web.Message{En: "Delete", It: "Elimina"}
	msgDeleted       = web.Message{En: "Deleted %s.", It: "Eliminato: %s."}
	msgUnitFixed     = web.Message{
		En: "Its Movements are recorded in %s, so its unit stays, even counted to nothing.",
		It: "I suoi Movimenti sono registrati in %s, quindi l'unità resta, anche contato a zero.",
	}
	msgUnitInRecipe   = web.Message{En: "Used by %s, so its unit stays.", It: "In uso in %s, quindi l'unità resta."}
	msgDeleteInRecipe = web.Message{En: "Used by %s, so it can't be deleted.", It: "In uso in %s, quindi non si può eliminare."}
	msgOneMoreRecipe  = web.Message{En: "%s and one more Recipe", It: "%s e un'altra Ricetta"}
	msgMoreRecipes    = web.Message{En: "%s and %d more Recipes", It: "%s e altre %d Ricette"}
	msgMovements      = web.Message{En: "Movements", It: "Movimenti"}
	msgStapleCounted  = web.Message{
		En: "%s counted to nothing: it's a Staple now.",
		It: "%s: contato a zero, ora è una Base.",
	}
	msgQuantityOf   = web.Message{En: "%s of %s", It: "%s di %s"}
	msgQuantityOfIn = web.Message{En: "%s of %s %s", It: "%s di %s %s"}
	msgQuantityIn   = web.Message{En: "%s %s", It: "%s %s"}
	msgAnd          = web.Message{En: "%s and %s", It: "%s e %s"}
	msgHasMovements = web.Message{
		En: "It has Movements, so it can't be deleted: the Kitchen Ledger keeps every one.",
		It: "Ha dei Movimenti, quindi non si può eliminare: il Registro della Cucina li conserva tutti.",
	}
	msgNameTaken = web.Message{
		En: "You already have %s: one food is one Ingredient.",
		It: "Hai già %s: un cibo è un solo Ingrediente.",
	}
)

// problemMessages say why a nutritional value is refused, where web.ProblemMessage's
// wording doesn't fit it.
var problemMessages = map[domain.Problem]web.Message{
	domain.Required: {En: "Required.", It: "Obbligatorio."},
	domain.TooLarge: {En: "Must be below 10000.", It: "Deve essere sotto 10000."},
}

// UnitNames name each Unit as a choice, recordedIn as quantities are recorded in it, and
// perUnit is what nutritional values are per.
var (
	UnitNames = map[domain.Unit]web.Message{
		domain.Grams:       {En: "g", It: "g"},
		domain.Millilitres: {En: "ml", It: "ml"},
		domain.Pieces:      {En: "piece", It: "pezzo"},
	}
	recordedIn = map[domain.Unit]web.Message{
		domain.Grams:       {En: "g", It: "g"},
		domain.Millilitres: {En: "ml", It: "ml"},
		domain.Pieces:      {En: "pieces", It: "pezzi"},
	}
	perUnit = map[domain.Unit]web.Message{
		domain.Grams:       {En: "per 100 g", It: "per 100 g"},
		domain.Millilitres: {En: "per 100 ml", It: "per 100 ml"},
		domain.Pieces:      {En: "per piece", It: "per pezzo"},
	}
)

// StorageLocationNames name each Storage Location.
var StorageLocationNames = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "pantry", It: "dispensa"},
	domain.Fridge:  {En: "fridge", It: "frigo"},
	domain.Freezer: {En: "freezer", It: "congelatore"},
}
