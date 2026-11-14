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
		En: "You have already written Ingredients, Recipes, Recipe Drafts, Portions, exclusions or allergies in this Language, so it stays.",
		It: "Hai già scritto Ingredienti, Ricette, Bozze di Ricetta, Porzioni, esclusioni o allergie in questa lingua, quindi resta questa.",
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
	msgTimesOfDay    = web.Message{En: "Times of day", It: "Orari"}
	msgAfter         = web.Message{En: "Must be after %s.", It: "Deve essere dopo l'ora di %s."}
	msgMealOut       = web.Message{En: "Meal Out estimate", It: "Stima di un Pasto Fuori"}
	msgMealOutHint   = web.Message{
		En: "What a Meal Out counts toward your Targets, unless you give it its own. Fibre and sugar may be left blank.",
		It: "Quanto conta un Pasto Fuori per i tuoi Obiettivi, se non gli dai una stima sua. Fibre e zuccheri si possono lasciare vuoti.",
	}
	msgCarbs     = web.Message{En: "Carbs", It: "Carboidrati"}
	msgFat       = web.Message{En: "Fat", It: "Grassi"}
	msgKcalUnit  = web.Message{En: "kcal", It: "kcal"}
	msgGrams     = web.Message{En: "g", It: "g"}
	msgFoods     = web.Message{En: "Exclusions and allergies", It: "Esclusioni e allergie"}
	msgFoodsHint = web.Message{
		En: "One food per line, or separated by commas or semicolons. Every planning Run is told to leave them out.",
		It: "Un alimento per riga, o separati da virgole o punti e virgola. Ogni Run di pianificazione è tenuto a escluderli.",
	}
	msgExclusions = web.Message{En: "Never eat", It: "Non mangio mai"}
	msgAllergies  = web.Message{En: "Allergies", It: "Allergie"}
	msgBrief      = web.Message{En: "Planning Brief", It: "Indicazioni per la pianificazione"}
	msgBriefHint  = web.Message{
		En: "Your lasting wishes for every Plan: variety, effort, what you enjoy. Every planning Run reads it and follows it as far as the rules of the Plan allow. Saving it starts no planning Run: it applies from the next one.",
		It: "I tuoi desideri durevoli per ogni Piano: varietà, impegno, cosa ti piace. Ogni Run di pianificazione li legge e li segue per quanto le regole del Piano lo permettono. Salvarli non avvia alcun Run: valgono dal prossimo.",
	}
	msgWishes           = web.Message{En: "Wishes", It: "Desideri"}
	msgBriefPlaceholder = web.Message{
		En: "Low-care Recipes, about 15 active minutes",
		It: "Ricette che chiedono poca cura, circa 15 minuti attivi",
	}
	msgSigningIn  = web.Message{En: "Signing in", It: "Accesso"}
	msgSignedInAs = web.Message{En: "Signed in as %s", It: "Accesso effettuato come %s"}
	msgSignOut    = web.Message{En: "Sign out", It: "Esci"}
)

// numberProblems say why a typed Target or Meal Out value is refused.
var numberProblems = map[domain.Problem]web.Message{
	domain.NotWhole:     {En: "Type a whole number.", It: "Scrivi un numero intero."},
	domain.NotAboveZero: {En: "Must be above 0.", It: "Deve essere sopra 0."},
	domain.TooLarge:     {En: "Must be below 10000.", It: "Deve essere sotto 10000."},
	domain.OutOfRange:   {En: "From 1 to 20.", It: "Da 1 a 20."},
}
