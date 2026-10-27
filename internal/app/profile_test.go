package app_test

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/simopzz/dandori/internal/apptest"
)

// validProfile is a complete Profile form: Targets, exclusions and
// allergies, the Meal Out default, the Untracked Allowance and the Slot
// times.
func validProfile() url.Values {
	return url.Values{
		"target_kcal":      {"2200"},
		"target_protein_g": {"150"},
		"target_carbs_g":   {"220"},
		"target_fat_g":     {"70"},

		"exclusions": {"Pork, mushrooms"},
		"allergies":  {"peanuts\nshellfish"},

		"meal_out_kcal":      {"900"},
		"meal_out_protein_g": {"35"},
		"meal_out_carbs_g":   {"90"},
		"meal_out_fat_g":     {"40"},

		"untracked_kcal":      {"150"},
		"untracked_protein_g": {"5"},
		"untracked_carbs_g":   {"15"},
		"untracked_fat_g":     {"5"},

		"breakfast_at": {"08:00"},
		"lunch_at":     {"13:00"},
		"snack_at":     {"17:00"},
		"dinner_at":    {"20:00"},
	}
}

// hasField reports whether page renders an input or textarea named name
// holding value.
func hasField(page, name, value string) bool {
	input := `name="` + name + `" value="` + value + `"`
	textarea := `name="` + name + `"`
	if strings.Contains(page, input) {
		return true
	}
	i := strings.Index(page, textarea)
	if i < 0 {
		return false
	}
	rest := page[i:]
	end := strings.Index(rest, "</textarea>")
	return end >= 0 && strings.Contains(rest[:end], ">"+value)
}

func TestProfileIsEmptyUntilFirstSaved(t *testing.T) {
	h := apptest.Start(t)

	page := h.Get(t, "/profile")
	if page.StatusCode != http.StatusOK {
		t.Fatalf("GET /profile status = %d, want 200", page.StatusCode)
	}
	if !hasField(page.Body, "target_kcal", "") {
		t.Errorf("a new Profile has no daily calorie Target to show:\n%s", page.Body)
	}
}

func TestSavedProfilePersists(t *testing.T) {
	h := apptest.Start(t)

	res := h.PostForm(t, "/profile", validProfile())
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Body, "Profile saved.") {
		t.Fatalf("POST /profile = %d, want 200 and a confirmation:\n%s", res.StatusCode, res.Body)
	}
	if res.Method != http.MethodGet {
		t.Errorf("a saved Profile is answered without a redirect, so reloading resubmits the form")
	}

	page := h.Get(t, "/profile").Body
	for name, value := range map[string]string{
		"target_kcal":         "2200",
		"target_protein_g":    "150",
		"target_carbs_g":      "220",
		"target_fat_g":        "70",
		"meal_out_kcal":       "900",
		"meal_out_protein_g":  "35",
		"meal_out_carbs_g":    "90",
		"meal_out_fat_g":      "40",
		"untracked_kcal":      "150",
		"untracked_protein_g": "5",
		"untracked_carbs_g":   "15",
		"untracked_fat_g":     "5",
	} {
		if !hasField(page, name, value) {
			t.Errorf("saved Profile does not show %s = %s", name, value)
		}
	}
	if !hasField(page, "exclusions", "pork\nmushrooms") {
		t.Errorf("saved Profile does not list the exclusions one per line:\n%s", page)
	}
	if !hasField(page, "allergies", "peanuts\nshellfish") {
		t.Errorf("saved Profile does not list the allergies one per line:\n%s", page)
	}
}

func TestEditingProfileReplacesItsTargetsAndExclusions(t *testing.T) {
	h := apptest.Start(t)
	h.PostForm(t, "/profile", validProfile())

	edited := validProfile()
	edited.Set("target_kcal", "1900")
	edited.Set("target_protein_g", "160")
	edited.Set("exclusions", "")
	edited.Set("allergies", "Peanuts, sesame")
	res := h.PostForm(t, "/profile", edited)
	if res.StatusCode != http.StatusOK || !strings.Contains(res.Body, "Profile saved.") {
		t.Fatalf("POST /profile = %d, want 200 and a confirmation:\n%s", res.StatusCode, res.Body)
	}

	page := h.Get(t, "/profile").Body
	if !hasField(page, "target_kcal", "1900") || !hasField(page, "target_protein_g", "160") {
		t.Errorf("edited Targets were not kept:\n%s", page)
	}
	if !hasField(page, "exclusions", "") || strings.Contains(page, "mushrooms") {
		t.Errorf("cleared exclusions are still shown:\n%s", page)
	}
	if !hasField(page, "allergies", "peanuts\nsesame") {
		t.Errorf("edited allergies were not kept:\n%s", page)
	}
}

func TestHTMXSubmissionSwapsOnlyTheProfileForm(t *testing.T) {
	h := apptest.Start(t)

	res := h.HTMXPostForm(t, "/profile", validProfile())
	if res.StatusCode != http.StatusOK {
		t.Fatalf("HTMX POST /profile status = %d, want 200", res.StatusCode)
	}
	if strings.Contains(res.Body, "<html") || !strings.Contains(res.Body, "Profile saved.") {
		t.Errorf("HTMX submission should get the saved form alone:\n%s", res.Body)
	}
}

func TestHTMXSubmissionWhileDatabaseIsUnreachableReloadsThePage(t *testing.T) {
	h := apptest.Start(t)
	h.CutDatabase(t)

	// Swapping the database-unavailable page into the form would nest it;
	// a full reload shows it properly.
	res := h.HTMXPostForm(t, "/profile", validProfile())
	if res.StatusCode != http.StatusServiceUnavailable || res.Header.Get("HX-Refresh") != "true" {
		t.Errorf("HTMX POST /profile while unreachable = %d, HX-Refresh %q; want 503 and a refresh", res.StatusCode, res.Header.Get("HX-Refresh"))
	}
}

func TestInvalidProfileIsRejectedAndNothingChanges(t *testing.T) {
	tests := []struct {
		name, field, value, message string
	}{
		{"missing calorie Target", "target_kcal", "", "Required"},
		{"zero calorie Target", "target_kcal", "0", "Must be more than 0"},
		{"not a number", "target_protein_g", "lots", "Must be a whole number"},
		{"fractional macro", "target_fat_g", "70.5", "Must be a whole number"},
		{"too large to store", "target_carbs_g", "99999999999", "Too large"},
		{"negative Meal Out estimate", "meal_out_kcal", "-10", "Can't be negative"},
		{"missing Untracked Allowance", "untracked_carbs_g", "", "Required"},
		{"Untracked Allowance uses the whole calorie Target", "untracked_kcal", "2000", "Must be less than the daily calorie Target"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := apptest.Start(t)
			saved := validProfile()
			saved.Set("target_kcal", "2000")
			h.PostForm(t, "/profile", saved)

			invalid := validProfile()
			invalid.Set("target_kcal", "2000")
			invalid.Set("exclusions", "anchovies")
			invalid.Set(tt.field, tt.value)
			res := h.PostForm(t, "/profile", invalid)

			if res.StatusCode != http.StatusUnprocessableEntity {
				t.Fatalf("POST /profile status = %d, want 422:\n%s", res.StatusCode, res.Body)
			}
			if !strings.Contains(html.UnescapeString(res.Body), tt.message) {
				t.Errorf("rejection does not say %q:\n%s", tt.message, res.Body)
			}
			if !hasField(res.Body, tt.field, tt.value) || !hasField(res.Body, "exclusions", "anchovies") {
				t.Errorf("rejected form does not keep what was typed:\n%s", res.Body)
			}
			if !strings.Contains(res.Body, `name="`+tt.field+`" value="`+tt.value+`" aria-invalid="true"`) {
				t.Errorf("rejected field is not marked invalid for assistive technology:\n%s", res.Body)
			}
			if strings.Contains(res.Body, "Profile saved.") {
				t.Errorf("rejected form claims the Profile was saved")
			}
			if page := h.Get(t, "/profile").Body; strings.Contains(page, "anchovies") {
				t.Errorf("rejected submission changed the stored Profile:\n%s", page)
			}
		})
	}
}

func TestAppShellLinksToTheProfile(t *testing.T) {
	h := apptest.Start(t)

	if page := h.Get(t, "/").Body; !strings.Contains(page, `href="/profile"`) {
		t.Errorf("app shell does not link to the Profile:\n%s", page)
	}
}

func TestProfileHoldsATimeOfDayPerSlotKind(t *testing.T) {
	h := apptest.Start(t)
	page := h.Get(t, "/profile").Body
	for name, value := range map[string]string{"breakfast_at": "08:00", "lunch_at": "13:00", "snack_at": "17:00", "dinner_at": "20:00"} {
		if !hasField(page, name, value) {
			t.Errorf("a new Profile does not offer %s = %s", name, value)
		}
	}

	profile := validProfile()
	profile.Set("lunch_at", "12:30")
	h.PostForm(t, "/profile", profile)
	if page := h.Get(t, "/profile").Body; !hasField(page, "lunch_at", "12:30") {
		t.Errorf("saved Profile does not show lunch at 12:30:\n%s", page)
	}

	profile.Set("dinner_at", "16:00")
	res := h.PostForm(t, "/profile", profile)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be after snack") {
		t.Errorf("dinner before snack = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
	profile.Set("dinner_at", "late")
	if res := h.PostForm(t, "/profile", profile); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(text(res.Body), "Must be a time") {
		t.Errorf("dinner at no time = %d, want 422 saying why:\n%s", res.StatusCode, text(res.Body))
	}
}
