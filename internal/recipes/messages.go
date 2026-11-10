package recipes

import (
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// The Recipe Library's Messages.
var (
	msgRecipes      = web.Message{En: "Recipes", It: "Ricette"}
	msgRecipesTitle = web.Message{En: "Recipes · Dandori", It: "Ricette · Dandori"}
	msgNoRecipes    = web.Message{En: "No Recipes yet.", It: "Ancora nessuna Ricetta."}
	msgSearch       = web.Message{En: "Search", It: "Cerca"}
	msgNoMatch      = web.Message{En: "No Recipe is like %s.", It: "Nessuna Ricetta somiglia a %s."}
	msgWrite        = web.Message{En: "Write a Recipe", It: "Scrivi una Ricetta"}
	msgWriteTitle   = web.Message{En: "Write a Recipe · Dandori", It: "Scrivi una Ricetta · Dandori"}
	msgRecipeTitle  = web.Message{En: "%s · Dandori", It: "%s · Dandori"}
	msgAdded        = web.Message{En: "Added %s to the Recipe Library.", It: "%s aggiunta al Ricettario."}
	msgSave         = web.Message{En: "Save Recipe", It: "Salva la Ricetta"}
	msgSaved        = web.Message{En: "Saved %s.", It: "Salvata: %s."}
	msgEdit         = web.Message{En: "Edit", It: "Modifica"}
	msgEditHeading  = web.Message{En: "Edit a Recipe", It: "Modifica una Ricetta"}
	msgEditTitle    = web.Message{En: "Edit a Recipe · Dandori", It: "Modifica una Ricetta · Dandori"}
	msgDelete       = web.Message{En: "Delete", It: "Elimina"}
	msgDeleted      = web.Message{En: "Deleted %s.", It: "Eliminata: %s."}

	msgPaste         = web.Message{En: "Paste a recipe", It: "Incolla una ricetta"}
	msgPasteHint     = web.Message{En: "in any language", It: "in qualsiasi lingua"}
	msgRead          = web.Message{En: "Read it", It: "Leggila"}
	msgPasteRequired = web.Message{En: "Paste a recipe to read.", It: "Incolla una ricetta da leggere."}
	ParseKindName    = web.Message{En: "Reading a Recipe", It: "Lettura di una Ricetta"}
	PastedName       = web.Message{En: "Pasted text", It: "Testo incollato"}
	DraftName        = web.Message{En: "Recipe Draft", It: "Bozza di Ricetta"}
	msgDraftTitle    = web.Message{En: "Recipe Draft · Dandori", It: "Bozza di Ricetta · Dandori"}
	msgStaple        = web.Message{En: "Staple", It: "Base"}

	msgName       = web.Message{En: "Name", It: "Nome"}
	msgPortions   = web.Message{En: "Portions", It: "Porzioni"}
	msgActive     = web.Message{En: "Active minutes", It: "Minuti attivi"}
	msgTotal      = web.Message{En: "Total minutes", It: "Minuti totali"}
	msgSlots      = web.Message{En: "Eaten at", It: "Si mangia a"}
	msgAppliances = web.Message{En: "Appliances", It: "Elettrodomestici"}
	msgOnePerLine = web.Message{En: "one per line", It: "uno per riga"}
	msgIngredient = web.Message{En: "Ingredient", It: "Ingrediente"}
	msgQuantity   = web.Message{En: "Quantity", It: "Quantità"}
	msgStep       = web.Message{En: "Step", It: "Passaggio"}

	msgIngredients   = web.Message{En: "Ingredients", It: "Ingredienti"}
	msgSteps         = web.Message{En: "Steps", It: "Passaggi"}
	msgPerPortion    = web.Message{En: "Per Portion", It: "Per Porzione"}
	msgAddIngredient = web.Message{En: "Add an Ingredient", It: "Aggiungi un Ingrediente"}
	msgAddStep       = web.Message{En: "Add a step", It: "Aggiungi un passaggio"}
	msgRowsHint      = web.Message{
		En: "Clear a row's name and quantity to remove it.",
		It: "Svuota nome e quantità di una riga per toglierla.",
	}
	msgMeasuredIn = web.Message{En: "Measured in %s", It: "Si misura in %s"}
	msgNew        = web.Message{En: "New", It: "Nuovo"}
	msgConfirmNew = web.Message{
		En: "Some Ingredients are new: check their values below, then save.",
		It: "Alcuni Ingredienti sono nuovi: controlla i loro valori qui sotto, poi salva.",
	}

	msgNameRequired     = web.Message{En: "Give it a name.", It: "Dalle un nome."}
	msgNameTaken        = web.Message{En: "You already have a Recipe with this name:", It: "Hai già una Ricetta con questo nome:"}
	msgPortionsRefused  = web.Message{En: "Must be a whole number of at least 1.", It: "Deve essere un numero intero, almeno 1."}
	msgPortionsTooLarge = web.Message{En: "Must be at most 99.", It: "Deve essere al massimo 99."}
	msgMinutesRequired  = web.Message{En: "Say how many minutes.", It: "Indica quanti minuti."}
	msgMinutesRefused   = web.Message{En: "Must be a whole number of minutes.", It: "Deve essere un numero intero di minuti."}
	msgMinutesTooLarge  = web.Message{En: "Must be at most 10080 minutes.", It: "Deve essere al massimo 10080 minuti."}
	msgTotalAboveZero   = web.Message{En: "Must be more than 0.", It: "Deve essere più di 0."}
	msgAboveTotal       = web.Message{En: "Can't be more than the total minutes.", It: "Non può superare i minuti totali."}
	msgSlotsRequired    = web.Message{En: "Choose at least one kind of Slot.", It: "Scegli almeno un tipo di Pasto."}
	msgNoIngredients    = web.Message{En: "List at least one Ingredient.", It: "Elenca almeno un Ingrediente."}

	msgMinutes      = web.Message{En: "%d min", It: "%d min"}
	msgKcal         = web.Message{En: "%s kcal", It: "%s kcal"}
	msgKcalPer      = web.Message{En: "%s kcal per Portion", It: "%s kcal per Porzione"}
	msgGrams        = web.Message{En: "%s g", It: "%s g"}
	msgAtLeastGrams = web.Message{En: "at least %s g", It: "almeno %s g"}
	msgUnknown      = web.Message{En: "unknown", It: "non noto"}
	msgProtein      = web.Message{En: "protein", It: "proteine"}
	msgCarbs        = web.Message{En: "carbs", It: "carboidrati"}
	msgFat          = web.Message{En: "fat", It: "grassi"}
	msgFibre        = web.Message{En: "fibre", It: "fibre"}
	msgSugar        = web.Message{En: "sugar", It: "zuccheri"}
	msgMakes        = web.Message{En: "Makes", It: "Dosi"}
	msgMakesValue   = web.Message{En: "%d Portions", It: "%d Porzioni"}
	msgMakesOne     = web.Message{En: "1 Portion", It: "1 Porzione"}
	msgActiveShort  = web.Message{En: "Active", It: "Attivi"}
	msgTotalShort   = web.Message{En: "Total", It: "Totale"}
	msgNeeds        = web.Message{En: "Needs", It: "Richiede"}
)

// fieldMessage says why field of the Recipe is refused for problem.
func fieldMessage(field string, problem domain.Problem) web.Message {
	switch {
	case field == "name":
		return msgNameRequired
	case field == "slots":
		return msgSlotsRequired
	case field == "ingredients":
		return msgNoIngredients
	case field == "portions" && problem == domain.TooLarge:
		return msgPortionsTooLarge
	case field == "portions":
		return msgPortionsRefused
	case problem == domain.Required:
		return msgMinutesRequired
	case problem == domain.TooLarge:
		return msgMinutesTooLarge
	case problem == domain.NotAboveZero:
		return msgTotalAboveZero
	case problem == domain.AboveTotal:
		return msgAboveTotal
	}
	return msgMinutesRefused
}

// quantityMessages say why a row's quantity is refused.
var quantityMessages = map[domain.Problem]web.Message{
	domain.Required:     {En: "Say how much.", It: "Indica quanto."},
	domain.NotAboveZero: {En: "Must be more than 0.", It: "Deve essere più di 0."},
	domain.NotWhole:     {En: "Whole grams and millilitres only.", It: "Solo grammi e millilitri interi."},
	domain.TooLarge:     {En: "Must be below 1000000.", It: "Deve essere sotto 1000000."},
}

// rowNameMessages say why a row's name is refused.
var rowNameMessages = map[domain.Problem]web.Message{
	domain.Required:    {En: "Name the Ingredient.", It: "Indica l'Ingrediente."},
	domain.ListedTwice: {En: "Already listed in this Recipe.", It: "Già presente in questa Ricetta."},
}
