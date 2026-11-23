package today

import (
	"github.com/simopzz/dandori/internal/planning"
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
	msgOpenCycle    = web.Message{En: "Open a Cycle", It: "Apri un Ciclo"}
	msgOpenNext     = web.Message{En: "Open the next Cycle", It: "Apri il prossimo Ciclo"}
	msgOrderingNext = web.Message{En: "Order the next Delivery", It: "Ordina la prossima Consegna"}
	msgNeedsTargets = web.Message{En: "Planning needs your Targets.", It: "Per pianificare servono i tuoi Obiettivi."}
	msgFailed       = web.Message{En: "Failed", It: "Fallita"} // as the Runs pages say it
)

// How planning a Cycle stands, of the current Cycle and of the next.
var (
	msgCurrentUnplanned = web.Message{En: "The current Cycle is waiting to be planned", It: "Il Ciclo in corso aspetta di essere pianificato"}
	msgNextUnplanned    = web.Message{En: "The next Cycle is waiting to be planned", It: "Il prossimo Ciclo aspetta di essere pianificato"}
	msgCurrentGoing     = web.Message{En: "The current Cycle is being planned", It: "Il Ciclo in corso è in pianificazione"}
	msgNextGoing        = web.Message{En: "The next Cycle is being planned", It: "Il prossimo Ciclo è in pianificazione"}
	msgCurrentProposal  = web.Message{En: "A Plan Proposal for the current Cycle is waiting", It: "Una Proposta di Piano per il Ciclo in corso ti aspetta"}
	msgNextProposal     = web.Message{En: "A Plan Proposal for the next Cycle is waiting", It: "Una Proposta di Piano per il prossimo Ciclo ti aspetta"}
	msgCurrentFailed    = web.Message{En: "Planning the current Cycle", It: "Pianificazione del Ciclo in corso"}
	msgNextFailed       = web.Message{En: "Planning the next Cycle", It: "Pianificazione del prossimo Ciclo"}
)

// offers are what Today says of each Offer about opening a Cycle, where it links and
// the icon leading it, its page's.
var offers = map[Offer]struct {
	text       web.Message
	href, icon string
}{
	OpenFirst:    {msgOpenCycle, "/plan/open", "calendar"},
	OpenNext:     {msgOpenNext, "/plan/open", "calendar"},
	TargetsFirst: {msgNeedsTargets, "/profile#targets", "user"},
}

// standings are what Today says of each Standing of a Cycle still to plan, of the
// current Cycle and of the next.
var standings = map[planning.Standing]struct{ current, next web.Message }{
	planning.Unplanned:       {msgCurrentUnplanned, msgNextUnplanned},
	planning.BeingPlanned:    {msgCurrentGoing, msgNextGoing},
	planning.ProposalWaiting: {msgCurrentProposal, msgNextProposal},
	planning.PlanningFailed:  {msgCurrentFailed, msgNextFailed},
}

// setUp is what planning needs, each named for what the User does at its link.
var setUp = []struct {
	href string
	text web.Message
}{
	{"/kitchen/count", msgCountKitchen},
	{"/recipes", msgAddRecipes},
	{"/profile#targets", msgSetTargets},
}
