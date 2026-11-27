package app_test

import (
	"testing"

	"github.com/mxschmitt/playwright-go"

	"github.com/simopzz/dandori/internal/apptest"
)

// Today shows each of the day's Slots with its time, the Portion filling it and where it
// comes from: the yogurt set out at breakfast, the Rice bowl cooked then at lunch and
// eaten again at dinner from the fridge, and the lasagne already in the kitchen.
func TestTodayShowsEachSlotsPortionAndWhereItComesFrom(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 07:00")
	s.Open("/")
	s.Sees("Next: breakfast at 8:00 am yogurt Breakfast 8:00 am set out 150 g yogurt")
	s.Sees("Later today Rice bowl Lunch 1:00 pm cooked then start at 12:35 pm")
	s.Sees("lasagne Snack 5:00 pm already in the kitchen")
	s.Sees("Rice bowl Dinner 8:00 pm from lunch, in the fridge")
	s.DoesNotSee("Start preparing")
}

// Opened, a Prep Session of a Recipe lists its Ingredients scaled to it, each with where
// it is taken from, and links to the Recipe for its steps.
func TestTodaysPrepSessionOpensToItsIngredientsAndRecipe(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.At("Fri 07:00")
	s.Open("/")
	s.Sees("Rice bowl rice 375 g in the pantry The Recipe's steps")
	s.Follows("The Recipe's steps")
	s.Sees("Rinse the rice. Cook it.")
}

// From a Prep Session's start time until its Slot's, a line at the top of Today says to
// start preparing it, also while another Slot is now: with breakfast at 12:20, lunch's
// Rice bowl starts at 12:35 while breakfast is now.
func TestTodaySaysToStartPreparingFromTheStartTimeToTheSlots(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.Open("/profile")
	s.SavesTimesOfDay(apptest.TimesOfDay{Breakfast: "12:20", Lunch: "13:00", Snack: "17:00", Dinner: "20:00"})
	approvesFriday(t, s)

	s.At("Fri 12:34")
	s.Open("/")
	s.DoesNotSee("Start preparing")
	s.At("Fri 12:35")
	s.Open("/")
	s.Sees("Start preparing Rice bowl for lunch 01 Now: breakfast at 12:20 pm yogurt Breakfast 12:20 pm set out 150 g yogurt")
	s.At("Fri 13:00")
	s.Open("/")
	s.DoesNotSee("Start preparing")
}

// A Slot whose Portion the kitchen no longer holds says so: the lasagne counted away
// isn't in the kitchen for the snack.
func TestATodaySlotWhosePortionWasCountedAwayIsNotInTheKitchen(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)

	s.CountsPortions(countLasagne, "0", "")
	s.At("Fri 07:00")
	s.Open("/")
	s.Sees("lasagne Snack 5:00 pm not in the kitchen")
	s.DoesNotSee("already in the kitchen")
}

// Today's main path in a browser: the line to start preparing the lunch, and its Slot, a
// click away among the later ones, opening to its Ingredients and where each is taken
// from, with a link to the Recipe.
func TestBrowserOpensTodaysPrepSession(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	approvesFriday(t, s)
	s.At("Fri 12:40")
	b := s.Browser(t)
	b.SignIn(t, "ada@example.com")
	b.Open(t, "/")
	b.SeesInTurn(t, "Start preparing Rice bowl for lunch", "Next: lunch at 1:00 pm", "Rice bowl", "cooked then", "start at 12:35 pm")

	expect := playwright.NewPlaywrightAssertions()
	pantry := b.GetByText("in the pantry")
	if err := expect.Locator(pantry).Not().ToBeVisible(); err != nil {
		t.Errorf("the Prep Session's Ingredients show before it is opened: %v", err)
	}
	if err := b.Locator("summary", playwright.PageLocatorOptions{HasText: "cooked then"}).Click(); err != nil {
		t.Fatalf("open the lunch's Slot: %v", err)
	}
	if err := expect.Locator(pantry).ToBeVisible(); err != nil {
		t.Errorf("opened, the lunch's Slot doesn't show where the rice is taken from: %v", err)
	}
	b.Shot(t, "today-prep-session")
	if err := b.GetByRole("link", playwright.PageGetByRoleOptions{Name: "The Recipe's steps"}).Click(); err != nil {
		t.Fatalf("follow the link to the Recipe: %v", err)
	}
	b.IsAt(t, "/recipes/1")
	b.SeesInTurn(t, "Steps", "Rinse the rice.", "Cook it.")
}
