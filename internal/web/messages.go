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
	domain.NotATime:   {En: "Type a time, like 1:00 pm or 13:00.", It: "Scrivi un'ora, come 13:00."},
	domain.NotADate:   {En: "Type a date, like 11/02/2026.", It: "Scrivi una data, come 02/11/2026."},
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

// ApplianceKindNames are each ApplianceKind's name, as a page shows it.
var ApplianceKindNames = map[domain.ApplianceKind]Message{
	domain.Oven:           {En: "oven", It: "forno"},
	domain.Hob:            {En: "hob", It: "piano cottura"},
	domain.Microwave:      {En: "microwave", It: "microonde"},
	domain.AirFryer:       {En: "air fryer", It: "friggitrice ad aria"},
	domain.PressureCooker: {En: "pressure cooker", It: "pentola a pressione"},
	domain.SlowCooker:     {En: "slow cooker", It: "slow cooker"},
	domain.Blender:        {En: "blender", It: "frullatore"},
	domain.FoodProcessor:  {En: "food processor", It: "robot da cucina"},
	domain.StandMixer:     {En: "stand mixer", It: "planetaria"},
	domain.Grill:          {En: "grill", It: "griglia"},
	domain.Toaster:        {En: "toaster", It: "tostapane"},
	domain.Steamer:        {En: "steamer", It: "vaporiera"},
}

// SlotKindChoices are the kinds of Slot as choices, in the order of a day.
func SlotKindChoices(ctx context.Context) []Choice {
	return kindChoices(ctx, domain.SlotKinds, SlotKindNames, false)
}

// ApplianceKindChoices are the kinds of appliance as choices, in the list's order, each
// with its icon.
func ApplianceKindChoices(ctx context.Context) []Choice {
	return kindChoices(ctx, domain.ApplianceKinds, ApplianceKindNames, true)
}

// kindChoices are all as choices named by names, with icons named by their codes when
// iconed says so.
func kindChoices[K ~string](ctx context.Context, all []K, names map[K]Message, iconed bool) []Choice {
	choices := make([]Choice, len(all))
	for i, k := range all {
		choices[i] = Choice{Value: string(k), Label: T(ctx, names[k])}
		if iconed {
			choices[i].Icon = string(k)
		}
	}
	return choices
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
