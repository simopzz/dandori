package domain

import "strings"

// ApplianceKind is a kind of kitchen appliance, from a fixed list (ADR-0020): what a
// Recipe needs and the Profile owns.
type ApplianceKind string

const (
	Oven           ApplianceKind = "oven"
	Hob            ApplianceKind = "hob"
	Microwave      ApplianceKind = "microwave"
	AirFryer       ApplianceKind = "air_fryer"
	PressureCooker ApplianceKind = "pressure_cooker"
	SlowCooker     ApplianceKind = "slow_cooker"
	Blender        ApplianceKind = "blender"
	FoodProcessor  ApplianceKind = "food_processor"
	StandMixer     ApplianceKind = "stand_mixer"
	Grill          ApplianceKind = "grill"
	Toaster        ApplianceKind = "toaster"
	Steamer        ApplianceKind = "steamer"
)

// ApplianceKinds lists every ApplianceKind in the order pages show them.
var ApplianceKinds = []ApplianceKind{
	Oven, Hob, Microwave, AirFryer, PressureCooker, SlowCooker,
	Blender, FoodProcessor, StandMixer, Grill, Toaster, Steamer,
}

// ParseApplianceKind reads an ApplianceKind from its code, reporting whether it is one
// on the list.
func ParseApplianceKind(code string) (ApplianceKind, bool) {
	for _, k := range ApplianceKinds {
		if string(k) == code {
			return k, true
		}
	}
	return "", false
}

// Appliances are the kinds of appliance a User owns, each with their free-text note on
// its model ("Instant Pot Duo 6 L"), which the planner reads and nothing compares.
type Appliances map[ApplianceKind]string

// ParseAppliances are the kinds whose codes are ticked, each with its note in notes
// trimmed. A note on a kind left unticked is dropped, and a code that isn't a kind
// is ignored.
func ParseAppliances(ticked []string, notes map[ApplianceKind]string) Appliances {
	owned := Appliances{}
	for _, code := range ticked {
		if k, ok := ParseApplianceKind(code); ok {
			owned[k] = strings.TrimSpace(notes[k])
		}
	}
	return owned
}
