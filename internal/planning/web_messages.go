package planning

import (
	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// The Cycle page's Messages, and opening and dropping a Cycle's.
var (
	msgCycle         = web.Message{En: "Cycle", It: "Ciclo"}
	msgCycleTitle    = web.Message{En: "Cycle · Dandori", It: "Ciclo · Dandori"}
	msgUpcomingCycle = web.Message{En: "Upcoming Cycle", It: "Prossimo Ciclo"}
	msgDelivery      = web.Message{En: "Delivery", It: "Consegna"}
	msgWindow        = web.Message{En: "%s, %s to %s", It: "%s, dalle %s alle %s"}
	msgTentative     = web.Message{En: "Tentative", It: "Provvisoria"}
	msgEndDate       = web.Message{En: "End date", It: "Data di fine"}
	msgNoCycle       = web.Message{
		En: "No Cycle is upcoming. Open one to plan the stretch your next Delivery has to last.",
		It: "Nessun Ciclo in arrivo. Aprine uno per pianificare il periodo che la prossima Consegna deve coprire.",
	}
	msgOpenCycle      = web.Message{En: "Open a Cycle", It: "Apri un Ciclo"}
	msgOpenCycleTitle = web.Message{En: "Open a Cycle · Dandori", It: "Apri un Ciclo · Dandori"}
	msgOpenHint       = web.Message{
		En: "A Cycle lasts until its end date: ten days after its Delivery unless you choose another, up to 14.",
		It: "Un Ciclo dura fino alla data di fine: dieci giorni dopo la Consegna, se non ne scegli un'altra, fino a 14.",
	}
	msgDeliveryDate = web.Message{En: "Delivery date", It: "Data della Consegna"}
	msgWindowFrom   = web.Message{En: "Window from", It: "Fascia dalle"}
	msgWindowTo     = web.Message{En: "Window to", It: "Fascia alle"}
	msgOptional     = web.Message{En: "optional", It: "facoltativa"}
	msgOpen         = web.Message{En: "Open the Cycle", It: "Apri il Ciclo"}
	msgOpened       = web.Message{En: "Cycle opened.", It: "Ciclo aperto."}
	msgUpcoming     = web.Message{En: "A Cycle is already upcoming. Drop it to open another.", It: "C'è già un Ciclo in arrivo. Annullalo per aprirne un altro."}
	msgSeeUpcoming  = web.Message{En: "See the upcoming Cycle", It: "Vedi il prossimo Ciclo"}
	msgNoTargets    = web.Message{En: "Planning needs your Targets.", It: "Per pianificare servono i tuoi Obiettivi."}
	msgSetTargets   = web.Message{En: "Set your Targets", It: "Imposta i tuoi Obiettivi"}
	msgDropCycle    = web.Message{En: "Drop the Cycle", It: "Annulla il Ciclo"}
	msgDropAsk      = web.Message{En: "Drop the Cycle whose Delivery comes on %s?", It: "Annullare il Ciclo con la Consegna di %s?"}
	msgDrop         = web.Message{En: "Drop", It: "Elimina"}
	msgDropped      = web.Message{En: "Cycle dropped.", It: "Ciclo annullato."}
)

// The Kitchen Check's Messages, the planning Run's and the Plan Proposal's.
var (
	msgPlanCycle    = web.Message{En: "Plan the Cycle", It: "Pianifica il Ciclo"}
	msgBeingPlanned = web.Message{En: "Being planned: see its Trace", It: "In pianificazione: vedi la Traccia"}
	msgKitchenCheck = web.Message{En: "Kitchen Check", It: "Controllo della cucina"}
	msgCheckTitle   = web.Message{En: "Kitchen Check · Dandori", It: "Controllo della cucina · Dandori"}
	msgCheckHint    = web.Message{
		En: "The Cycle of the Delivery on %s is planned from what the kitchen holds now. Count what has drifted, then plan.",
		It: "Il Ciclo della Consegna di %s è pianificato da ciò che la cucina contiene ora. Conta ciò che è cambiato, poi pianifica.",
	}
	msgConfirmAndPlan = web.Message{En: "Confirm the kitchen and plan", It: "Conferma la cucina e pianifica"}
	msgBusy           = web.Message{
		En: "Another planning Run is going, or a Plan Proposal is waiting for you. Plan once it is decided.",
		It: "C'è già un Run di pianificazione in corso, o una Proposta di Piano che ti aspetta. Pianifica quando sarà decisa.",
	}
	msgSeeCycle     = web.Message{En: "See the Cycle", It: "Vedi il Ciclo"}
	msgPlanProposal = web.Message{En: "Plan Proposal", It: "Proposta di Piano"}
	msgPortions     = web.Message{En: "%s: %d Portions", It: "%s: %d Porzioni"}
	msgOnePortion   = web.Message{En: "%s: 1 Portion", It: "%s: 1 Porzione"}
	msgPortionsAt   = web.Message{En: "%s: %d Portions at %s", It: "%s: %d Porzioni a %s"}
	msgOnePortionAt = web.Message{En: "%s: 1 Portion at %s", It: "%s: 1 Porzione a %s"}
	msgSize         = web.Message{En: "%s×", It: "%s×"}
	msgSlotOn       = web.Message{En: "%s on %s", It: "%s di %s"}
	msgMinutes      = web.Message{En: "%d active of %d minutes", It: "%d attivi su %d minuti"}
	msgNoSlot       = web.Message{En: "No Slot", It: "Nessun Pasto"}

	// KindName and ProposalName tell a planning Run's page its kind and what it leaves.
	KindName     = web.Message{En: "Planning a Cycle", It: "Pianificazione di un Ciclo"}
	ProposalName = web.Message{En: "Plan Proposal", It: "Proposta di Piano"}
)

// Reasons are why a planning Run fails beside the reasons every Run can; %s is what was
// said more, verbatim.
var Reasons = map[agent.Reason]web.Message{
	agent.NoResult: {En: "The model ended without a Plan Proposal.", It: "Il modello ha finito senza una Proposta di Piano."},
	BrokeHardRules: {
		En: "The planner submitted three Plans breaking hard rules:\n%s",
		It: "Il pianificatore ha proposto tre Piani che violano regole rigide:\n%s",
	},
}

// cycleProblems say why a field of opening a Cycle is refused, in its own words.
var cycleProblems = map[domain.Problem]web.Message{
	domain.NotAfterFrom:     {En: "Must be after the window's start.", It: "Deve essere dopo l'inizio della fascia."},
	domain.AlreadyEnded:     {En: "This window has already ended.", It: "Questa fascia è già finita."},
	domain.NotAfterDelivery: {En: "Must be after the Delivery's date.", It: "Deve essere dopo la data della Consegna."},
	domain.TooLong:          {En: "At most 14 days after the Delivery's date.", It: "Al massimo 14 giorni dopo la data della Consegna."},
}
