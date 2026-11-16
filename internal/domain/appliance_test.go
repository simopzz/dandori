package domain_test

import (
	"maps"
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

// The appliances owned are the kinds ticked, each with its note on one line: a note on a
// kind left unticked is dropped, and a code that isn't a kind is ignored.
func TestTheAppliancesOwnedAreTheKindsTicked(t *testing.T) {
	tests := []struct {
		name   string
		ticked []string
		notes  map[domain.ApplianceKind]string
		want   domain.Appliances
	}{
		{name: "none ticked", want: domain.Appliances{}},
		{name: "ticked without notes", ticked: []string{"oven", "air_fryer"}, want: domain.Appliances{domain.Oven: "", domain.AirFryer: ""}},
		{
			name: "a ticked kind's note on one line", ticked: []string{"pressure_cooker"},
			notes: map[domain.ApplianceKind]string{domain.PressureCooker: "  Instant Pot\r\nDuo  6 L "},
			want:  domain.Appliances{domain.PressureCooker: "Instant Pot Duo 6 L"},
		},
		{
			name: "an unticked kind's note dropped", ticked: []string{"hob"},
			notes: map[domain.ApplianceKind]string{domain.Hob: "induction", domain.Oven: "Bosch"},
			want:  domain.Appliances{domain.Hob: "induction"},
		},
		{name: "an unknown code ignored", ticked: []string{"ice_cream_maker", "hob"}, want: domain.Appliances{domain.Hob: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.ParseAppliances(tt.ticked, tt.notes); !maps.Equal(got, tt.want) {
				t.Errorf("ParseAppliances(%q, %q) = %q, want %q", tt.ticked, tt.notes, got, tt.want)
			}
		})
	}
}
