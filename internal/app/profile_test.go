package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Each Profile section saves on its own. Its button shows only Save, and its accessible
// name says what it saves, in the User's Language.
func TestEachProfileSectionsButtonNamesIt(t *testing.T) {
	shows := map[string]string{"English": "Save", "Italiano": "Salva"}
	want := map[string]map[string]string{
		"English": {
			"profile-language":   "Save Language",
			"profile-times":      "Save the times of day",
			"profile-targets":    "Save Targets",
			"profile-meal-out":   "Save the Meal Out estimate",
			"profile-foods":      "Save exclusions and allergies",
			"profile-appliances": "Save appliances",
			"profile-brief":      "Save the Planning Brief",
		},
		"Italiano": {
			"profile-language":   "Salva la Lingua",
			"profile-times":      "Salva gli orari",
			"profile-targets":    "Salva gli Obiettivi",
			"profile-meal-out":   "Salva la stima di un Pasto Fuori",
			"profile-foods":      "Salva esclusioni e allergie",
			"profile-appliances": "Salva gli elettrodomestici",
			"profile-brief":      "Salva le indicazioni per la pianificazione",
		},
	}
	for language, buttons := range want {
		t.Run(language, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			s.ChangesLanguage(language)
			s.Open("/profile")
			for id, name := range buttons {
				f := apptest.Form(t, s.Page().Body, id)
				if f.Button != shows[language] {
					t.Errorf("%s's button shows %q, want %q", id, f.Button, shows[language])
				}
				if f.ButtonName != name {
					t.Errorf("%s's button is named %q, want %q", id, f.ButtonName, name)
				}
			}
		})
	}
}
