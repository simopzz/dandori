package domain

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
