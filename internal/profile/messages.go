package profile

import "github.com/simopzz/dandori/internal/web"

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
		En: "Your kitchen already holds Ingredients or Recipes in this Language, so it stays.",
		It: "La tua cucina ha già Ingredienti o Ricette in questa lingua, quindi resta questa.",
	}
	msgProfile      = web.Message{En: "Profile", It: "Profilo"}
	msgProfileTitle = web.Message{En: "Profile · Dandori", It: "Profilo · Dandori"}
	msgSave         = web.Message{En: "Save", It: "Salva"}
	msgSaved        = web.Message{En: "Profile saved.", It: "Profilo salvato."}
	msgSigningIn    = web.Message{En: "Signing in", It: "Accesso"}
	msgSignedInAs   = web.Message{En: "Signed in as %s", It: "Accesso effettuato come %s"}
	msgSignOut      = web.Message{En: "Sign out", It: "Esci"}
)
