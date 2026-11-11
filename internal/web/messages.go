package web

import (
	"context"

	"github.com/simopzz/dandori/internal/domain"
)

// The shell's and the shared components' Messages.
var (
	msgRecipes        = Message{En: "Recipes", It: "Ricette"}
	msgKitchen        = Message{En: "Kitchen", It: "Cucina"}
	msgProfile        = Message{En: "Profile", It: "Profilo"}
	msgRuns           = Message{En: "Runs", It: "Run"}
	msgMainNav        = Message{En: "Main", It: "Principale"}
	msgLanguageSwitch = Message{En: "Language", It: "Lingua"}
	msgChange         = Message{En: "Change", It: "Modifica"}
	msgChooseAdd      = Message{En: "Choose what to add", It: "Scegli cosa aggiungere"}
	msgClose          = Message{En: "Close", It: "Chiudi"}
	msgCloseNotice    = Message{En: "Close notice", It: "Chiudi avviso"}
)

// problems say why a typed number is refused, the same for every field.
var problems = map[domain.Problem]Message{
	domain.NotANumber: {En: "Type a number.", It: "Scrivi un numero."},
	domain.Negative:   {En: "Can't be below 0.", It: "Non può essere sotto 0."},
	domain.TooPrecise: {En: "One decimal place at most.", It: "Al massimo un decimale."},
}

// ProblemMessage says why a typed value is refused for problem: as own says, a field's
// own wording, or else as every field does.
func ProblemMessage(problem domain.Problem, own map[domain.Problem]Message) Message {
	if m, ok := own[problem]; ok {
		return m
	}
	return problems[problem]
}

// SlotKindNames are each SlotKind's name, as a page shows it.
var SlotKindNames = map[domain.SlotKind]Message{
	domain.Breakfast: {En: "breakfast", It: "colazione"},
	domain.Lunch:     {En: "lunch", It: "pranzo"},
	domain.Snack:     {En: "snack", It: "spuntino"},
	domain.Dinner:    {En: "dinner", It: "cena"},
}

// InLocations say where in each Storage Location, as Italian takes each Storage
// Location with its own preposition.
var InLocations = map[domain.StorageLocation]Message{
	domain.Pantry:  {En: "in the pantry", It: "in dispensa"},
	domain.Fridge:  {En: "in the fridge", It: "in frigo"},
	domain.Freezer: {En: "in the freezer", It: "nel congelatore"},
}

// unitQuantities write a quantity of each Unit.
var unitQuantities = map[domain.Unit]Message{
	domain.Grams:       {En: "%s g", It: "%s g"},
	domain.Millilitres: {En: "%s ml", It: "%s ml"},
	domain.Pieces:      {En: "%s pieces", It: "%s pezzi"},
	domain.PortionUnit: {En: "%s", It: "%s"},
}

// msgOnePiece is one piece, which alone isn't plural.
var msgOnePiece = Message{En: "1 piece", It: "1 pezzo"}

// Quantity is q of unit as the request's Language writes it.
func Quantity(ctx context.Context, q domain.Quantity, unit domain.Unit) string {
	if unit == domain.Pieces && q == domain.QuantityOf(1) {
		return T(ctx, msgOnePiece)
	}
	return Tf(ctx, unitQuantities[unit], Number(ctx, q.Number()))
}
