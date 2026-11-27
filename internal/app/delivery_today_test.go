package app_test

import "testing"

// On its day, until it arrives, Today says a tentative Delivery is due, with its window
// and that it isn't marked ordered yet, linking its Grocery List; the day before it says
// nothing of it.
func TestTodaySaysATentativeDeliveryIsDue(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	s.OpensCycle(friday)
	s.At("Wed 23:59")
	s.Open("/")
	s.DoesNotSee("Delivery today")

	s.At("Thu 00:00")
	s.Open("/")
	s.Sees("Delivery today, 6:00 pm to 8:00 pm not marked ordered yet")
	s.Follows("Delivery today, 6:00 pm to 8:00 pm")
	s.IsAt("/plan/cycles/1/grocery-list")
}

// An ordered Delivery's line says only its window, until the window ends, when the
// put-away list takes its place.
func TestTodaySaysAnOrderedDeliveryIsDueUntilItArrives(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.Follows("Grocery List")
	s.Submits("mark-ordered")
	s.IsConfirmed("Delivery marked ordered.")

	s.At("Thu 19:59")
	s.Open("/")
	s.Sees("Delivery today, 6:00 pm to 8:00 pm")
	s.DoesNotSee("not marked ordered yet")

	s.At("Thu 20:00")
	s.Open("/")
	s.DoesNotSee("Delivery today")
	s.Sees("Put the Delivery away")
}

// On a planned, tentative Delivery's day its line stands in for the reminder to order it,
// which links the same Grocery List.
func TestADeliveryDueStandsInForTheReminderToOrderIt(t *testing.T) {
	s := plannerWithKitchen(t, "ada@example.com")
	plansFridaySplitting(t, s)
	s.At("Wed 12:00")
	s.Open("/")
	s.Sees("Order the next Delivery")

	s.At("Thu 08:00")
	s.Open("/")
	s.Sees("Delivery today, 6:00 pm to 8:00 pm not marked ordered yet")
	s.DoesNotSee("Order the next Delivery")
}

// The line is told in the User's Language.
func TestTodaySaysADeliveryIsDueInItalian(t *testing.T) {
	s := plannerOf(t, "ada@example.com")
	s.OpensCycle(friday)
	s.Open("/profile")
	s.ChangesLanguage("Italiano")
	s.At("Thu 08:00")
	s.Open("/")
	s.Sees("Consegna oggi, dalle 18:00 alle 20:00 non ancora segnata come ordinata")
}
