package app_test

import (
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// Each Profile section saves on its own, so its button names what it saves, in the
// User's Language.
func TestEachProfileSectionsButtonNamesIt(t *testing.T) {
	want := map[string]map[string]string{
		"English": {
			"profile-language": "Save Language",
			"profile-times":    "Save the times of day",
			"profile-targets":  "Save Targets",
			"profile-meal-out": "Save the Meal Out estimate",
			"profile-foods":    "Save exclusions and allergies",
			"profile-brief":    "Save the Planning Brief",
		},
		"Italiano": {
			"profile-language": "Salva la Lingua",
			"profile-times":    "Salva gli orari",
			"profile-targets":  "Salva gli Obiettivi",
			"profile-meal-out": "Salva la stima di un Pasto Fuori",
			"profile-foods":    "Salva esclusioni e allergie",
			"profile-brief":    "Salva le indicazioni per la pianificazione",
		},
	}
	for language, buttons := range want {
		t.Run(language, func(t *testing.T) {
			s := profileOf(t, "ada@example.com")
			s.ChangesLanguage(language)
			s.Open("/profile")
			for id, button := range buttons {
				if got := apptest.Form(t, s.Page().Body, id).Button; got != button {
					t.Errorf("%s's button reads %q, want %q", id, got, button)
				}
			}
		})
	}
}
