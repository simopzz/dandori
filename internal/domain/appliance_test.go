package domain_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

// The kinds of appliance are a fixed list, in ADR-0020's order.
func TestTheApplianceKindsAreAFixedList(t *testing.T) {
	want := []domain.ApplianceKind{
		"oven", "hob", "microwave", "air_fryer", "pressure_cooker", "slow_cooker",
		"blender", "food_processor", "stand_mixer", "grill", "toaster", "steamer",
	}
	if !slices.Equal(domain.ApplianceKinds, want) {
		t.Errorf("ApplianceKinds = %v, want %v", domain.ApplianceKinds, want)
	}
}

// A kind of appliance is read from its code, and only a code on the list is one.
func TestAnApplianceKindIsParsedFromItsCode(t *testing.T) {
	for code, want := range map[string]domain.ApplianceKind{"oven": domain.Oven, "air_fryer": domain.AirFryer, "steamer": domain.Steamer} {
		if got, ok := domain.ParseApplianceKind(code); !ok || got != want {
			t.Errorf("ParseApplianceKind(%q) = %q, %v; want %q, true", code, got, ok, want)
		}
	}
	for _, code := range []string{"", "air fryer", "Oven", "ice_cream_maker", "forno"} {
		if got, ok := domain.ParseApplianceKind(code); ok {
			t.Errorf("ParseApplianceKind(%q) = %q, true; want no kind", code, got)
		}
	}
}
