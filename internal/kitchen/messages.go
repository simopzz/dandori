package kitchen

import (
	"context"

	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// The kitchen's Messages.
var (
	msgKitchen      = web.Message{En: "Kitchen", It: "Cucina"}
	msgKitchenTitle = web.Message{En: "Kitchen · Dandori", It: "Cucina · Dandori"}
	msgEmptyKitchen = web.Message{
		En: "Nothing in your kitchen yet. Count what you have, or add an Ingredient to start.",
		It: "Ancora niente nella tua cucina. Conta quello che hai, o aggiungi un Ingrediente per cominciare.",
	}
	msgUpToDateAt = web.Message{En: "Kitchen up to date as of %s", It: "Cucina aggiornata alle %s"}
	msgUpToDateOn = web.Message{En: "Kitchen up to date as of %s, %s", It: "Cucina aggiornata %s alle %s"}

	msgCount          = web.Message{En: "Count", It: "Conta"}
	msgCountTitle     = web.Message{En: "Count · Dandori", It: "Conta · Dandori"}
	msgCounted        = web.Message{En: "Counted %s: %s %s.", It: "Contato %s: %s %s."}
	msgCountedAt      = web.Message{En: "counted at %s", It: "contato alle %s"}
	msgCountedOn      = web.Message{En: "counted on %s", It: "contato %s"}
	msgSearch         = web.Message{En: "Search your Ingredients", It: "Cerca tra i tuoi Ingredienti"}
	msgNoMatch        = web.Message{En: "No Ingredient is like %s.", It: "Nessun Ingrediente somiglia a %s."}
	msgNothingToCount = web.Message{En: "No Ingredients to count yet.", It: "Ancora nessun Ingrediente da contare."}
	msgWhere          = web.Message{En: "Where it is, and what Dandori believes is there", It: "Dov'è, e quanto Dandori crede che ce ne sia"}
	msgHolds          = web.Message{En: "%s %s", It: "%s %s"}
	msgStated         = web.Message{En: "How much is there", It: "Quanto ce n'è"}
	msgNote           = web.Message{En: "Note", It: "Nota"}
	msgOptional       = web.Message{En: "optional", It: "facoltativa"}
	msgAnother        = web.Message{En: "Choose another", It: "Scegline un altro"}

	msgMove         = web.Message{En: "Move", It: "Sposta"}
	msgMoveTitle    = web.Message{En: "Move · Dandori", It: "Sposta · Dandori"}
	msgMoved        = web.Message{En: "Moved %s of %s %s.", It: "Spostato: %s di %s %s."}
	msgMoveQuantity = web.Message{En: "How much to move", It: "Quanto spostare"}
	msgMoveWhere    = web.Message{En: "Where it goes, and what Dandori believes is there", It: "Dove va, e quanto Dandori crede che ce ne sia"}
	msgChooseTarget = web.Message{En: "Choose where it goes.", It: "Scegli dove va."}
	msgMoreThanHeld = web.Message{
		En: "Dandori believes only %s is here. If there is more, count it first.",
		It: "Dandori crede che qui ce ne sia solo %s. Se ce n'è di più, prima conta.",
	}

	msgMovements      = web.Message{En: "Movements", It: "Movimenti"}
	msgMovementsTitle = web.Message{En: "Movements · Dandori", It: "Movimenti · Dandori"}
	msgLocations      = web.Message{En: "Where it is kept", It: "Dove si tiene"}
	msgNoMovements    = web.Message{En: "No Movements here yet.", It: "Ancora nessun Movimento qui."}
	msgHeldIn         = web.Message{En: "%s · %s", It: "%s · %s"}
	msgBeforeAfter    = web.Message{En: "%s → %s", It: "%s → %s"}
	msgHappened       = web.Message{En: "%s %s", It: "%s %s"}

	msgAddPortions         = web.Message{En: "Add Portions", It: "Aggiungi Porzioni"}
	msgAddPortionsTitle    = web.Message{En: "Add Portions · Dandori", It: "Aggiungi Porzioni · Dandori"}
	msgAddedPortions       = web.Message{En: "Added %s × %d %s.", It: "Aggiunto: %s × %d %s."}
	msgPortionName         = web.Message{En: "Name", It: "Nome"}
	msgPortionNameRequired = web.Message{En: "Give it a name.", It: "Dagli un nome."}
	msgHowMany             = web.Message{En: "How many", It: "Quante"}
	msgPortionWhere        = web.Message{En: "Where", It: "Dove"}
	msgPortionValuesHint   = web.Message{
		En: "Per Portion: calories in kcal, the rest in grams. Fibre and sugar may be left blank.",
		It: "Per Porzione: calorie in kcal, il resto in grammi. Fibre e zuccheri si possono lasciare vuoti.",
	}
	msgAdd        = web.Message{En: "Add", It: "Aggiungi"}
	msgPerPortion = web.Message{En: "per Portion", It: "per Porzione"}
	msgTimes      = web.Message{En: "× %d", It: "× %d"}

	msgLotHolds             = web.Message{En: "This Portion Lot holds %d.", It: "Questo Lotto di Porzioni ne contiene %d."}
	msgMovedPortions        = web.Message{En: "Moved %s × %d %s.", It: "Spostato: %s × %d %s."}
	msgMovePortionsQuantity = web.Message{En: "How many to move", It: "Quante spostare"}
	msgPortionsMoreThanHeld = web.Message{
		En: "This Portion Lot holds %d. If there are more, count them first.",
		It: "Questo Lotto di Porzioni ne contiene %d. Se sono di più, prima contale.",
	}

	msgChooseWhere = web.Message{En: "Choose where it is kept.", It: "Scegli dove si tiene."}
	msgStaple      = web.Message{En: "%s is a Staple: it is never counted.", It: "%s è una Base: non si conta mai."}
)

// problemMessages say why a stated quantity is refused, where web.ProblemMessage's
// wording doesn't fit it.
var problemMessages = map[domain.Problem]web.Message{
	domain.Required: {En: "Say how much is there.", It: "Di' quanto ce n'è."},
	domain.NotWhole: {En: "Grams and millilitres are whole numbers.", It: "Grammi e millilitri sono numeri interi."},
	domain.TooLarge: {En: "Must be below 1000000.", It: "Deve essere sotto 1000000."},
}

// moveProblemMessages say why a quantity to move is refused, where web.ProblemMessage's
// wording doesn't fit it. More moved than is held needs what is held: msgMoreThanHeld
// says that.
var moveProblemMessages = map[domain.Problem]web.Message{
	domain.Required:     {En: "Say how much to move.", It: "Di' quanto spostare."},
	domain.NothingMoved: {En: "Say how much to move.", It: "Di' quanto spostare."},
	domain.NotWhole:     {En: "Grams and millilitres are whole numbers.", It: "Grammi e millilitri sono numeri interi."},
	domain.TooLarge:     {En: "Must be below 1000000.", It: "Deve essere sotto 1000000."},
}

// howManyProblemMessages say why how many Portions to add is refused.
var howManyProblemMessages = map[domain.Problem]web.Message{
	domain.Required:     {En: "Say how many.", It: "Di' quante."},
	domain.NotWhole:     {En: "Must be a whole number of at least 1.", It: "Deve essere un numero intero, almeno 1."},
	domain.TooLarge:     {En: "Must be at most 99.", It: "Deve essere al massimo 99."},
	domain.TooManyAlike: {En: "That would make more than 99 alike here.", It: "Così sarebbero più di 99 uguali qui."},
}

// portionCountProblemMessages say why a Count of a Lot is refused.
var portionCountProblemMessages = map[domain.Problem]web.Message{
	domain.Required: {En: "Say how many.", It: "Di' quante."},
	domain.NotWhole: {En: "Must be a whole number.", It: "Deve essere un numero intero."},
	domain.TooLarge: {En: "Must be at most 99.", It: "Deve essere al massimo 99."},
}

// portionMoveProblemMessages say why how many of a Lot to move is refused. More than
// it holds needs what it holds: msgPortionsMoreThanHeld says that.
var portionMoveProblemMessages = map[domain.Problem]web.Message{
	domain.Required:     {En: "Say how many to move.", It: "Di' quante spostare."},
	domain.NothingMoved: {En: "Say how many to move.", It: "Di' quante spostare."},
	domain.NotWhole:     {En: "Must be a whole number.", It: "Deve essere un numero intero."},
	domain.TooLarge:     {En: "Must be below 1000000.", It: "Deve essere sotto 1000000."},
	domain.TooManyAlike: {En: "That would make more than 99 alike here.", It: "Così sarebbero più di 99 uguali qui."},
}

// locationHeadings head each Storage Location's Stock Items.
var locationHeadings = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "Pantry", It: "Dispensa"},
	domain.Fridge:  {En: "Fridge", It: "Frigo"},
	domain.Freezer: {En: "Freezer", It: "Congelatore"},
}

// movedToLocations and movedFromLocations tell a move's cause on the Stock Item it left
// and the one it reached, and toLocations where a move went, as Italian takes each
// Storage Location with its own preposition.
var (
	movedToLocations = map[domain.StorageLocation]web.Message{
		domain.Pantry:  {En: "Moved to the pantry", It: "Spostato in dispensa"},
		domain.Fridge:  {En: "Moved to the fridge", It: "Spostato in frigo"},
		domain.Freezer: {En: "Moved to the freezer", It: "Spostato nel congelatore"},
	}
	movedFromLocations = map[domain.StorageLocation]web.Message{
		domain.Pantry:  {En: "Moved from the pantry", It: "Spostato dalla dispensa"},
		domain.Fridge:  {En: "Moved from the fridge", It: "Spostato dal frigo"},
		domain.Freezer: {En: "Moved from the freezer", It: "Spostato dal congelatore"},
	}
	toLocations = map[domain.StorageLocation]web.Message{
		domain.Pantry:  {En: "to the pantry", It: "in dispensa"},
		domain.Fridge:  {En: "to the fridge", It: "in frigo"},
		domain.Freezer: {En: "to the freezer", It: "nel congelatore"},
	}
)

// causeNames tell the cause of a Movement, which the Kitchen Ledger stores as data.
var causeNames = map[domain.Cause]web.Message{
	domain.Counted:      {En: "Counted", It: "Contato"},
	domain.MarkedStaple: {En: "Marked a Staple", It: "Segnato come Base"},
	domain.Added:        {En: "Added", It: "Aggiunte"},
}

// causeText tells the cause of m as a Movement of the Stock Item in location.
func causeText(ctx context.Context, m domain.StockMovement, location domain.StorageLocation) string {
	switch {
	case m.Cause != domain.Moved:
		return web.T(ctx, causeNames[m.Cause])
	case m.To == location:
		return web.T(ctx, movedFromLocations[m.Key.Location])
	}
	return web.T(ctx, movedToLocations[m.To])
}
