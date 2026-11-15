package today

import (
	"github.com/simopzz/dandori/internal/web"
)

// Today's Messages.
var (
	msgToday          = web.Message{En: "Today", It: "Oggi"}
	msgTodayTitle     = web.Message{En: "Today · Dandori", It: "Oggi · Dandori"}
	msgNowAt          = web.Message{En: "Now: %s at %s", It: "Ora: %s alle %s"}
	msgNextAt         = web.Message{En: "Next: %s at %s", It: "A seguire: %s alle %s"}
	msgNextTomorrowAt = web.Message{En: "Next: %s tomorrow at %s", It: "A seguire: %s domani alle %s"}
	msgLaterToday     = web.Message{En: "Later today", It: "Più tardi oggi"}
	msgNoPlan         = web.Message{
		En: "Your Plan shows here once Dandori plans what you eat. To plan it, it needs to know your kitchen, your Recipes and your Targets.",
		It: "Il tuo Piano compare qui quando Dandori pianifica cosa mangi. Per pianificarlo, deve conoscere la tua cucina, le tue Ricette e i tuoi Obiettivi.",
	}
	msgCountKitchen = web.Message{En: "Count your kitchen", It: "Conta la tua cucina"}
	msgAddRecipes   = web.Message{En: "Add Recipes", It: "Aggiungi Ricette"}
	msgSetTargets   = web.Message{En: "Set your Targets", It: "Imposta i tuoi Obiettivi"}
)

// setUp is what planning needs, each named for what the User does at its link.
var setUp = []struct {
	href string
	text web.Message
}{
	{"/kitchen/count", msgCountKitchen},
	{"/recipes", msgAddRecipes},
	{"/profile#targets", msgSetTargets},
}
