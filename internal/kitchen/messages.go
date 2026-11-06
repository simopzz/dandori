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
	msgCountedOn      = web.Message{En: "counted on %s, %s", It: "contato %s alle %s"}
	msgSearch         = web.Message{En: "Search your Ingredients", It: "Cerca tra i tuoi Ingredienti"}
	msgNoMatch        = web.Message{En: "No Ingredient is like %s.", It: "Nessun Ingrediente somiglia a %s."}
	msgNothingToCount = web.Message{En: "No Ingredients to count yet.", It: "Ancora nessun Ingrediente da contare."}
	msgWhere          = web.Message{En: "Where it is, and what Dandori believes is there", It: "Dov'è, e quanto Dandori crede che ce ne sia"}
	msgHolds          = web.Message{En: "%s %s", It: "%s %s"}
	msgStated         = web.Message{En: "How much is there", It: "Quanto ce n'è"}
	msgNote           = web.Message{En: "Note", It: "Nota"}
	msgOptional       = web.Message{En: "optional", It: "facoltativa"}
	msgAnother        = web.Message{En: "Choose another", It: "Scegline un altro"}

	msgMovements      = web.Message{En: "Movements", It: "Movimenti"}
	msgMovementsTitle = web.Message{En: "Movements · Dandori", It: "Movimenti · Dandori"}
	msgMovementsIn    = web.Message{En: "Movements %s", It: "Movimenti %s"}
	msgStockItem      = web.Message{En: "%s · %s", It: "%s · %s"}
	msgBeforeAfter    = web.Message{En: "%s → %s", It: "%s → %s"}
	msgHappened       = web.Message{En: "%s, %s", It: "%s alle %s"}

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

// locationHeadings head each Storage Location's Stock Items.
var locationHeadings = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "Pantry", It: "Dispensa"},
	domain.Fridge:  {En: "Fridge", It: "Frigo"},
	domain.Freezer: {En: "Freezer", It: "Congelatore"},
}

// inLocations say where in each Storage Location, as Italian takes each with its own
// preposition.
var inLocations = map[domain.StorageLocation]web.Message{
	domain.Pantry:  {En: "in the pantry", It: "in dispensa"},
	domain.Fridge:  {En: "in the fridge", It: "in frigo"},
	domain.Freezer: {En: "in the freezer", It: "nel congelatore"},
}

// causeNames tell the cause of a Movement, which the Kitchen Ledger stores as data.
var causeNames = map[domain.Cause]web.Message{
	domain.Counted: {En: "Counted", It: "Contato"},
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
