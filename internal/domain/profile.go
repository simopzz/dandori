package domain

// Profile is a User's stored settings: their Language, the time of day of each kind of
// Slot, their Targets, the estimate of a Meal Out, the foods they exclude and are
// allergic to, and the appliances they own. Their Planning Brief isn't part of it.
type Profile struct {
	Language              Language
	SlotTimes             Times
	Targets               Targets
	MealOut               Nutrition
	Exclusions, Allergies []string
	Appliances            Appliances
}
