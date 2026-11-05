package kitchen

import "github.com/simopzz/dandori/internal/web"

// The kitchen's Messages.
var (
	msgKitchen      = web.Message{En: "Kitchen", It: "Cucina"}
	msgKitchenTitle = web.Message{En: "Kitchen · Dandori", It: "Cucina · Dandori"}
	msgEmptyKitchen = web.Message{
		En: "Nothing in your kitchen yet. Add your Ingredients to start.",
		It: "Ancora niente nella tua cucina. Aggiungi i tuoi Ingredienti per cominciare.",
	}
	msgUpToDateAt = web.Message{En: "Kitchen up to date as of %s", It: "Cucina aggiornata alle %s"}
	msgUpToDateOn = web.Message{En: "Kitchen up to date as of %s, %s", It: "Cucina aggiornata %s alle %s"}
)
