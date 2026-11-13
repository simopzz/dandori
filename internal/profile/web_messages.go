package profile

import (
	"github.com/simopzz/dandori/internal/domain"
	"github.com/simopzz/dandori/internal/web"
)

// The Profile's Messages.
var (
	msgChooseLanguage = web.Message{En: "Choose your Language", It: "Scegli la tua lingua"}
	msgChooseTitle    = web.Message{En: "Language · Dandori", It: "Lingua · Dandori"}
	msgChooseLede     = web.Message{
		En: "Dandori speaks it on every page, and writes every Ingredient and Recipe in it.",
		It: "Dandori la parla in ogni pagina, e ci scrive ogni Ingrediente e Ricetta.",
	}
	msgChooseHint = web.Message{
		En: "You can change it on your Profile until your kitchen holds its first Ingredient or Recipe.",
		It: "Puoi cambiarla nel tuo Profilo finché la tua cucina non ha il suo primo Ingrediente o Ricetta.",
	}
	msgLanguage      = web.Message{En: "Language", It: "Lingua"}
	msgContinue      = web.Message{En: "Continue", It: "Continua"}
	msgChooseOne     = web.Message{En: "Choose English or Italiano.", It: "Scegli English o Italiano."}
	msgLanguageStays = web.Message{
		En: "Your kitchen already holds Ingredients, Recipes, Recipe Drafts or Portions in this Language, so it stays.",
		It: "La tua cucina ha già Ingredienti, Ricette, Bozze di Ricetta o Porzioni in questa lingua, quindi resta questa.",
	}
	msgParsing = web.Message{
		En: "A recipe is being read in this Language, so it stays until that ends.",
		It: "Una ricetta è in lettura in questa lingua, quindi resta questa finché non finisce.",
	}
	msgProfile       = web.Message{En: "Profile", It: "Profilo"}
	msgProfileTitle  = web.Message{En: "Profile · Dandori", It: "Profilo · Dandori"}
	msgSave          = web.Message{En: "Save", It: "Salva"}
	msgSaved         = web.Message{En: "Profile saved.", It: "Profilo salvato."}
	msgTargets       = web.Message{En: "Targets", It: "Obiettivi"}
	msgTargetsNeeded = web.Message{
		En: "Planning needs your Targets.",
		It: "Per pianificare servono i tuoi Obiettivi.",
	}
	msgCarbsAndFat = web.Message{
		En: "Carbs and fat have no Target: they are shown, never aimed at.",
		It: "Carboidrati e grassi non hanno un Obiettivo: si mostrano, senza puntare a un valore.",
	}
	msgKcal          = web.Message{En: "Calories", It: "Calorie"}
	msgKcalHint      = web.Message{En: "kcal a day", It: "kcal al giorno"}
	msgTolerance     = web.Message{En: "Tolerance", It: "Tolleranza"}
	msgToleranceHint = web.Message{En: "± %", It: "± %"}
	msgProtein       = web.Message{En: "Protein", It: "Proteine"}
	msgFibre         = web.Message{En: "Fibre", It: "Fibre"}
	msgSugar         = web.Message{En: "Sugar", It: "Zuccheri"}
	msgFloorHint     = web.Message{En: "g, at least", It: "g, almeno"}
	msgCeilingHint   = web.Message{En: "g, at most", It: "g, al massimo"}
	msgSigningIn     = web.Message{En: "Signing in", It: "Accesso"}
	msgSignedInAs    = web.Message{En: "Signed in as %s", It: "Accesso effettuato come %s"}
	msgSignOut       = web.Message{En: "Sign out", It: "Esci"}
)

// targetProblems say why a typed Target is refused.
var targetProblems = map[domain.Problem]web.Message{
	domain.Required:     {En: "Required.", It: "Obbligatorio."},
	domain.NotWhole:     {En: "Type a whole number.", It: "Scrivi un numero intero."},
	domain.NotAboveZero: {En: "Must be above 0.", It: "Deve essere sopra 0."},
	domain.TooLarge:     {En: "Must be below 10000.", It: "Deve essere sotto 10000."},
	domain.OutOfRange:   {En: "From 1 to 20.", It: "Da 1 a 20."},
}
