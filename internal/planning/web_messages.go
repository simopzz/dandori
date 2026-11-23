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
	msgCurrentCycle  = web.Message{En: "Current Cycle", It: "Ciclo in corso"}
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
	msgSeeCycle       = web.Message{En: "See the Cycle", It: "Vedi il Ciclo"}
	msgPlanProposal   = web.Message{En: "Plan Proposal", It: "Proposta di Piano"}
	msgPortions       = web.Message{En: "%s: %d Portions", It: "%s: %d Porzioni"}
	msgOnePortion     = web.Message{En: "%s: 1 Portion", It: "%s: 1 Porzione"}
	msgPortionsAt     = web.Message{En: "%s: %d Portions at %s", It: "%s: %d Porzioni a %s"}
	msgOnePortionAt   = web.Message{En: "%s: 1 Portion at %s", It: "%s: 1 Porzione a %s"}
	msgSize           = web.Message{En: "%s×", It: "%s×"}
	msgSlotOn         = web.Message{En: "%s on %s", It: "%s di %s"}
	msgMinutes        = web.Message{En: "%d active of %d minutes", It: "%d attivi su %d minuti"}
	msgNoSlot         = web.Message{En: "No Slot", It: "Nessun Pasto"}
	msgSlots          = web.Message{En: "Slots", It: "Pasti"}
	msgPrepSessions   = web.Message{En: "Prep Sessions", It: "Sessioni di preparazione"}
	msgToFreezer      = web.Message{En: "To the freezer at put-away", It: "Nel congelatore quando riponi la Consegna"}
	msgToFreezerCount = web.Message{En: "%d to the freezer at put-away", It: "%d nel congelatore quando riponi la Consegna"}
	msgIngredients    = web.Message{En: "Ingredients", It: "Ingredienti"}
	msgPortionsHead   = web.Message{En: "Portions", It: "Porzioni"}
	msgKcalUnit       = web.Message{En: "kcal", It: "kcal"}

	msgNothingToFreezer = web.Message{En: "Nothing to the freezer", It: "Niente nel congelatore"}

	msgPlanningKind = web.Message{En: "Planning a Cycle", It: "Pianificazione di un Ciclo"}
)

// Deciding on a Plan Proposal's Messages, the Plan's and a failed planning Run's.
var (
	msgPlan            = web.Message{En: "Plan", It: "Piano"}
	msgApprove         = web.Message{En: "Approve", It: "Approva"}
	msgApproved        = web.Message{En: "Plan approved.", It: "Piano approvato."}
	msgPlanningNote    = web.Message{En: "Planning Note", It: "Nota di pianificazione"}
	msgReject          = web.Message{En: "Reject", It: "Rifiuta"}
	msgRejected        = web.Message{En: "Plan Proposal rejected.", It: "Proposta di Piano rifiutata."}
	msgStartsInThePast = web.Message{
		En: "This Plan Proposal starts at a Slot that has passed, so it can't be approved. Reject it with a Planning Note to have it planned again, or reject it and plan the Cycle again.",
		It: "Questa Proposta di Piano inizia da un Pasto già passato, quindi non può essere approvata. Rifiutala con una Nota di pianificazione per ripianificarla, oppure rifiutala e pianifica di nuovo il Ciclo.",
	}
	msgPlanned     = web.Message{En: "This Cycle already has a Plan.", It: "Questo Ciclo ha già un Piano."}
	msgNothingLeft = web.Message{En: "Every Slot of this Cycle has passed: there's nothing left to plan.", It: "Tutti i Pasti di questo Ciclo sono passati: non resta nulla da pianificare."}
	msgFailed      = web.Message{En: "Planning failed: %s", It: "La pianificazione non è riuscita: %s"}
	msgSeeTrace    = web.Message{En: "See its Trace", It: "Vedi la Traccia"}
	msgPlanAgain   = web.Message{En: "Plan again", It: "Pianifica di nuovo"}

	msgAskForChanges = web.Message{En: "Ask for changes", It: "Chiedi modifiche"}
	msgAskHint       = web.Message{
		En: "Dandori plans the Cycle again from the next Slot, told your note. The Plan stays as it is until you approve the new Plan Proposal.",
		It: "Dandori pianifica di nuovo il Ciclo dal prossimo Pasto, seguendo la tua nota. Il Piano resta com'è finché non approvi la nuova Proposta di Piano.",
	}
	msgNoNote        = web.Message{En: "Say what to change.", It: "Scrivi cosa cambiare."}
	msgChangesFrom   = web.Message{En: "Changes from the Plan: %s", It: "Modifiche al Piano: %s"}
	msgPlanAgainHint = web.Message{
		En: "Dandori plans again, with or without a note. A note tells it what to change.",
		It: "Dandori pianifica di nuovo, con o senza una nota. Una nota gli dice cosa cambiare.",
	}
	msgCheckKitchenFirst = web.Message{En: "Check the kitchen first", It: "Controlla prima la cucina"}
	msgBeingPlannedTag   = web.Message{En: "Being planned", It: "In pianificazione"}
	msgSeeProgress       = web.Message{En: "See progress", It: "Vedi l'avanzamento"}
	msgTentativeHint     = web.Message{En: "the Delivery's date may still move", It: "la data della Consegna può ancora cambiare"}
	msgPlanningCycle     = web.Message{En: "Planning the Cycle of %s", It: "Pianificazione del Ciclo del %s"}
)

// The Messages of the links between the current and the upcoming Cycle.
var (
	msgCycles             = web.Message{En: "Cycles", It: "Cicli"}
	msgCurrentDates       = web.Message{En: "Current · %s", It: "In corso · %s"}
	msgUpcomingDates      = web.Message{En: "Upcoming · %s", It: "Prossimo · %s"}
	msgProposalWaitingTag = web.Message{En: "Plan Proposal waiting", It: "Proposta di Piano in attesa"}
	msgFailedTag          = web.Message{En: "Failed", It: "Fallita"} // as the Runs pages say it
)

// What cancelled a planning Run when the User didn't, on its page.
var (
	msgClosedBy     = web.Message{En: "The Delivery of %s arrived and closed its Cycle.", It: "La Consegna di %s è arrivata e ha chiuso il suo Ciclo."}
	msgCycleDropped = web.Message{En: "Its Cycle was dropped.", It: "Il suo Ciclo è stato annullato."}
)

// The Plan Proposal's soft misses' Messages, and the heads of its figures.
var (
	msgBackToBack   = web.Message{En: "%s eaten back to back, %s", It: "%s mangiato di seguito, %s"}
	msgAnd          = web.Message{En: "%s and %s", It: "%s e %s"}
	msgUnused       = web.Message{En: "%s: %d of %d left unused, %s", It: "%s: %d di %d non usate, %s"}
	msgMadeForAfter = web.Message{En: "%s: %d made for after the Cycle, %s", It: "%s: %d preparate per dopo il Ciclo, %s"}
	msgCaloriesWord = web.Message{En: "calories", It: "calorie"}
	msgProteinWord  = web.Message{En: "protein", It: "proteine"}
	msgFibreWord    = web.Message{En: "fibre", It: "fibre"}
	msgSugarWord    = web.Message{En: "sugar", It: "zuccheri"}

	msgCalories  = web.Message{En: "Calories, kcal", It: "Calorie, kcal"}
	msgProtein   = web.Message{En: "Protein, g", It: "Proteine, g"}
	msgFibre     = web.Message{En: "Fibre, g", It: "Fibre, g"}
	msgSugar     = web.Message{En: "Sugar, g", It: "Zuccheri, g"}
	msgAverage   = web.Message{En: "Average", It: "Media"}
	msgRange     = web.Message{En: "%s–%s", It: "%s–%s"}
	msgAtLeast   = web.Message{En: "at least", It: "almeno"}
	msgAtMost    = web.Message{En: "at most", It: "al massimo"}
	msgGramsUnit = web.Message{En: "g", It: "g"}
	msgUnderWord = web.Message{En: "under", It: "sotto"}
	msgOverWord  = web.Message{En: "over", It: "sopra"}
)

// planningReasons are why a planning Run fails beside the reasons every Run can; %s is
// what was said more, verbatim.
var planningReasons = map[agent.Reason]web.Message{
	agent.NoResult: {En: "The model ended without a Plan Proposal.", It: "Il modello ha finito senza una Proposta di Piano."},
	brokeHardRules: {
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

// The pending Plan Proposal's Messages: the Cycle's header, the averages and soft misses,
// rejecting, and the days.
var (
	msgDates       = web.Message{En: "%s – %s", It: "%s – %s"}
	msgPlannedFrom = web.Message{En: "planned from %s %s", It: "pianificato da %s, %s"}
	msgDays        = web.Message{En: "Days", It: "Giorni"}
	msgMarkRule    = web.Message{
		En: "A day is marked when it is more than 25%% off a Target, the average when it misses at all. + means some Ingredients have no value.",
		It: "Un giorno è segnato quando si scosta di oltre il 25%% da un Obiettivo, la media quando lo manca. + vuol dire che alcuni Ingredienti non hanno valori.",
	}
	msgOneDayOff     = web.Message{En: "1 day more than 25%% off", It: "1 giorno oltre il 25%%"}
	msgDaysOff       = web.Message{En: "%d days more than 25%% off", It: "%d giorni oltre il 25%%"}
	msgEarlierNotes  = web.Message{En: "Earlier Planning Notes", It: "Note di pianificazione precedenti"}
	msgRejectHint    = web.Message{En: "With a note, Dandori plans again. Without one, it only rejects.", It: "Con una nota, Dandori pianifica di nuovo. Senza, rifiuta soltanto."}
	msgToday         = web.Message{En: "Today", It: "Oggi"}
	msgOneDayPast    = web.Message{En: "1 day past", It: "1 giorno passato"}
	msgDaysPast      = web.Message{En: "%d days past", It: "%d giorni passati"}
	msgDaysBefore    = web.Message{En: "%d days before", It: "I %d giorni prima"}
	msgOnPlan        = web.Message{En: "%d on the current Plan", It: "%d nel Piano attuale"}
	msgWithNoPlan    = web.Message{En: "%d with no Plan", It: "%d senza Piano"}
	msgNoPlanOn      = web.Message{En: "%s: no Plan", It: "%s: nessun Piano"}
	msgFromSlot      = web.Message{En: "%s · %s→", It: "%s · %s→"}
	msgNothingCooked = web.Message{En: "Nothing to cook", It: "Niente da cucinare"}
	msgCooked        = web.Message{En: "%s · %d min active", It: "%s · %d min attivi"}
	msgInTheKitchen  = web.Message{En: "already in the kitchen", It: "già in cucina"}
	msgCookedThatDay = web.Message{En: "cooked", It: "cucinata"}
	msgCookedOn      = web.Message{En: "from %s, %s", It: "da %s, %s"}
	msgEatenLater    = web.Message{En: "Eaten later", It: "Mangiate più avanti"}
	msgPortionsSized = web.Message{En: "%d at %s", It: "%d da %s"}
	msgTotalActive   = web.Message{En: "%d min · %d active", It: "%d min · %d attivi"}
	msgAfterTheCycle = web.Message{En: "After the Cycle", It: "Dopo il Ciclo"}

	msgNewRecipe         = web.Message{En: "New Recipe", It: "Nuova Ricetta"}
	msgNewRecipeNamed    = web.Message{En: "New Recipe: %s", It: "Nuova Ricetta: %s"}
	msgNewRecipeSource   = web.Message{En: "new Recipe", It: "nuova Ricetta"}
	msgOneNewIngredient  = web.Message{En: "1 new Ingredient", It: "1 nuovo Ingrediente"}
	msgNewIngredients    = web.Message{En: "%d new Ingredients", It: "%d nuovi Ingredienti"}
	msgBackToTheProposal = web.Message{En: "Back to the Plan Proposal", It: "Torna alla Proposta di Piano"}
	msgClash             = web.Message{
		En: "The new Recipe %s no longer fits your Recipe Library or Ingredients, which have changed since it was drafted, so this Plan Proposal can't be approved. Reject it with a Planning Note to have it planned again.",
		It: "La nuova Ricetta %s non è più compatibile con il tuo Ricettario o i tuoi Ingredienti, cambiati da quando è stata scritta, quindi questa Proposta di Piano non può essere approvata. Rifiutala con una Nota di pianificazione per ripianificarla.",
	}
)

// The Grocery List's Messages.
var (
	msgGroceryList      = web.Message{En: "Grocery List", It: "Lista della spesa"}
	msgGroceryListTitle = web.Message{En: "Grocery List · Dandori", It: "Lista della spesa · Dandori"}
	msgGroceryHint      = web.Message{
		En: "What the Plan needs beyond what the kitchen will hold when the Delivery arrives. Tick each line as you fill the cart.",
		It: "Ciò che serve al Piano oltre a quanto la cucina avrà all'arrivo della Consegna. Spunta ogni riga mentre riempi il carrello.",
	}
	msgToBuy        = web.Message{En: "To buy", It: "Da comprare"}
	msgNothingToBuy = web.Message{En: "The kitchen will hold all the Plan needs.", It: "La cucina avrà tutto ciò che serve al Piano."}
	msgCheckYouHave = web.Message{En: "Check you still have", It: "Controlla di avere ancora"}
	msgPlanFirst    = web.Message{En: "Plan the Cycle first: its Grocery List follows its Plan.", It: "Prima pianifica il Ciclo: la sua Lista della spesa segue il suo Piano."}
)
