package planning

import (
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

// cycleProblems say why a field of opening a Cycle is refused, in its own words.
var cycleProblems = map[domain.Problem]web.Message{
	domain.NotAfterFrom:     {En: "Must be after the window's start.", It: "Deve essere dopo l'inizio della fascia."},
	domain.AlreadyEnded:     {En: "This window has already ended.", It: "Questa fascia è già finita."},
	domain.NotAfterDelivery: {En: "Must be after the Delivery's date.", It: "Deve essere dopo la data della Consegna."},
	domain.TooLong:          {En: "At most 14 days after the Delivery's date.", It: "Al massimo 14 giorni dopo la data della Consegna."},
}
