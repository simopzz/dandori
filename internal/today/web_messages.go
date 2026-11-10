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
	msgNoCycle        = web.Message{En: "No Cycle yet.", It: "Ancora nessun Ciclo."}
	msgOpenKitchen    = web.Message{En: "Open your kitchen", It: "Apri la tua cucina"}
)
