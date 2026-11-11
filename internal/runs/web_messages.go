package runs

import (
	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/web"
)

// The Runs' Messages: every fixed line of a Trace, in each Language.
var (
	msgRunTitle = web.Message{En: "%s · Dandori", It: "%s · Dandori"}
	msgStarted  = web.Message{En: "Started at %s", It: "Iniziata alle %s"}
	// msgStartedOn is a Run's day and time on the Runs page.
	msgStartedOn = web.Message{En: "%s %s", It: "%s %s"}
	msgTrace     = web.Message{En: "Trace", It: "Traccia"}
	msgRuns      = web.Message{En: "Runs", It: "Run"}
	msgNoRuns    = web.Message{En: "No Runs yet.", It: "Ancora nessuna Run."}
	msgCancel    = web.Message{En: "Cancel", It: "Annulla"}
	msgTryAgain  = web.Message{En: "Try again", It: "Riprova"}
	// msgSeeFurther opens a Run's page again, further along, without JavaScript.
	msgSeeFurther = web.Message{En: "See how it's going", It: "Guarda a che punto è"}

	msgStartedWith = web.Message{En: "Started with %s", It: "Iniziata con %s"}
	msgAsked       = web.Message{En: "Asking the model (turn %d)", It: "Chiedo al modello (turno %d)"}
	msgSaid        = web.Message{En: "The model says", It: "Il modello dice"}
	msgCalled      = web.Message{En: "Calls %s", It: "Chiama %s"}
	msgAnswered    = web.Message{En: "%s answers", It: "%s risponde"}
	msgRefused     = web.Message{En: "%s refuses", It: "%s rifiuta"}
	msgRetrying    = web.Message{En: "Retry %d of %d in %d ms", It: "Nuovo tentativo %d di %d tra %d ms"}
	msgTokens      = web.Message{
		En: "%d tokens in, %d out, %d of them thinking; %d read from the cache, %d written to it",
		It: "%d token in entrata, %d in uscita, di cui %d di ragionamento; %d letti dalla cache, %d scritti",
	}
	msgEnded = web.Message{En: "Ended: %s", It: "Finita: %s"}
)

// statusNames are each status of a Run.
var statusNames = map[Status]web.Message{
	Going:     {En: "Going", It: "In corso"},
	Succeeded: {En: "Succeeded", It: "Riuscita"},
	Failed:    {En: "Failed", It: "Fallita"},
	Cancelled: {En: "Cancelled", It: "Annullata"},
}

// reasonNames are why a Run fails; %s is what was said more, verbatim.
var reasonNames = map[agent.Reason]web.Message{
	agent.TurnLimit:            {En: "The model did not finish within %s turns.", It: "Il modello non ha finito in %s turni."},
	agent.ModelRefused:         {En: "The model declined to go on.", It: "Il modello si è rifiutato di continuare."},
	agent.OutOfTokens:          {En: "The model ran out of tokens before it finished.", It: "Il modello ha finito i token prima di concludere."},
	agent.OutOfTokensInThought: {En: "The model ran out of tokens while thinking.", It: "Il modello ha finito i token mentre ragionava."},
	agent.UnexpectedStop:       {En: "The model stopped for a reason Dandori doesn't know: %s", It: "Il modello si è fermato per un motivo che Dandori non conosce: %s"},
	agent.ModelFailed:          {En: "Asking the model failed: %s", It: "La richiesta al modello è fallita: %s"},
	// shortcut: names the parse Run's result, the only kind's; take it from the Kind once
	// another kind of Run can end without its result.
	agent.NoResult:    {En: "The model ended without a Recipe Draft.", It: "Il modello ha finito senza una Bozza di Ricetta."},
	agent.Crashed:     {En: "Dandori crashed while running it: %s", It: "Dandori si è bloccato: %s"},
	agent.NoModel:     {En: "No model is set up.", It: "Nessun modello è configurato."},
	agent.Interrupted: {En: "Dandori stopped while it was going.", It: "Dandori si è fermato mentre era in corso."},
}
