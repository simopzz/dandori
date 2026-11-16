package runs

import (
	"github.com/simopzz/dandori/internal/agent"
	"github.com/simopzz/dandori/internal/web"
)

// The Runs' Messages: every fixed line of a Trace, in each Language.
var (
	msgRunTitle = web.Message{En: "%s · Dandori", It: "%s · Dandori"}
	msgStarted  = web.Message{En: "Started at %s", It: "Iniziata alle %s"}
	msgTrace    = web.Message{En: "Trace", It: "Traccia"}
	msgRuns     = web.Message{En: "Runs", It: "Run"}
	msgNoRuns   = web.Message{En: "No Runs yet.", It: "Ancora nessuna Run."}
	msgCancel   = web.Message{En: "Cancel", It: "Annulla"}
	msgTryAgain = web.Message{En: "Try again", It: "Riprova"}
	// msgSeeFurther opens a Run's page again, further along, without JavaScript.
	msgSeeFurther = web.Message{En: "See how it's going", It: "Guarda a che punto è"}

	// A Trace Event's line says what happened in plain words; the model's and the tools'
	// names, and their data, are in its Fold.
	msgStartedRun = web.Message{En: "Started", It: "Iniziata"}
	msgAsked      = web.Message{En: "Asking the model (turn %d)", It: "Chiedo al modello (turno %d)"}
	msgSaid       = web.Message{En: "The model says", It: "Il modello dice"}
	msgCalled     = web.Message{En: "The model asks Dandori", It: "Il modello chiede a Dandori"}
	msgAnswered   = web.Message{En: "Dandori answers", It: "Dandori risponde"}
	msgRefused    = web.Message{En: "Dandori sends it back", It: "Dandori lo rimanda indietro"}
	msgRetrying   = web.Message{En: "Asking the model again (%d of %d)", It: "Chiedo di nuovo al modello (%d di %d)"}
	msgWaiting    = web.Message{En: "Waiting %d ms", It: "Attendo %d ms"}
	msgUsed       = web.Message{En: "The model used %d tokens", It: "Il modello ha usato %d token"}
	msgTokens     = web.Message{
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
	agent.Crashed:              {En: "Dandori crashed while running it: %s", It: "Dandori si è bloccato: %s"},
	agent.NoModel:              {En: "No model is set up.", It: "Nessun modello è configurato."},
	agent.Interrupted:          {En: "Dandori stopped while it was going.", It: "Dandori si è fermato mentre era in corso."},
}
