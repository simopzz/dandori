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
	msgMovementsIn    = web.Message{En: "Movements %s", It: "Movimenti %s"}
	msgStockItem      = web.Message{En: "%s · %s", It: "%s · %s"}
	msgBeforeAfter    = web.Message{En: "%s → %s", It: "%s → %s"}
	msgHappened       = web.Message{En: "%s %s", It: "%s %s"}

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

// locationHeadings head each Storage Location's Stock Items.
var locationHeadings = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "Pantry", It: "Dispensa"},
	domain.Fridge:  {En: "Fridge", It: "Frigo"},
	domain.Freezer: {En: "Freezer", It: "Congelatore"},
}

// inLocations say where in each Storage Location, as Italian takes each with its
// own preposition.
var inLocations = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "in the pantry", It: "in dispensa"},
	domain.Fridge:  {En: "in the fridge", It: "in frigo"},
	domain.Freezer: {En: "in the freezer", It: "nel congelatore"},
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
	domain.Counted: {En: "Counted", It: "Contato"},
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

// unitQuantities write a quantity of each Unit.
var unitQuantities = map[domain.Unit]web.Message{
	domain.Grams:       {En: "%s g", It: "%s g"},
	domain.Millilitres: {En: "%s ml", It: "%s ml"},
	domain.Pieces:      {En: "%s pieces", It: "%s pezzi"},
}

// msgOnePiece is one piece, which alone isn't plural.
var msgOnePiece = web.Message{En: "1 piece", It: "1 pezzo"}

// quantity is q of unit as the request's Language writes it.
func quantity(ctx context.Context, q domain.Quantity, unit domain.Unit) string {
	if unit == domain.Pieces && q == domain.QuantityOf(1) {
		return web.T(ctx, msgOnePiece)
	}
	return web.Tf(ctx, unitQuantities[unit], web.Number(ctx, q.Number()))
}
