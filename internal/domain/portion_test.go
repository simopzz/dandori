package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

func TestParseHowMany(t *testing.T) {
	cases := []struct {
		typed   string
		want    int
		problem domain.Problem
	}{
		{"1", 1, ""},
		{" 99 ", 99, ""},
		{"", 0, domain.Required},
		{"0", 0, domain.NotWhole},
		{"-2", 0, domain.NotWhole},
		{"1.5", 0, domain.NotWhole},
		{"a few", 0, domain.NotWhole},
		{"100", 0, domain.TooLarge},
	}
	for _, c := range cases {
		got, problem := domain.ParseHowMany(c.typed)
		if got != c.want || problem != c.problem {
			t.Errorf("ParseHowMany(%q) = %d, %q; want %d, %q", c.typed, got, problem, c.want, c.problem)
		}
	}
}

// Alike Portions share a group, spelled as the oldest; a different Storage Location or
// value makes another, and groups go by Storage Location, then name.
func TestGroupPortions(t *testing.T) {
	values := domain.Nutrition{Kcal: domain.Known(620), Protein: domain.Known(32), Carbs: domain.Known(58), Fat: domain.Known(28)}
	lighter := values
	lighter.Kcal = domain.Known(480)
	knownFibre := values
	knownFibre.Fibre = domain.Known(4)

	portions := []domain.Portion{
		{ID: 1, Name: "Lasagne", Nutrition: values},
		{ID: 2, Name: " lasagne ", Nutrition: values},
		{ID: 3, Name: "lasagne", Nutrition: values},
		{ID: 4, Name: "lasagne", Nutrition: lighter},
		{ID: 5, Name: "lasagne", Nutrition: knownFibre},
		{ID: 6, Name: "chili", Nutrition: values},
		{ID: 7, Name: "gone", Nutrition: values},
	}
	where := map[int64]domain.StorageLocation{
		1: domain.Freezer, 2: domain.Freezer, 3: domain.Fridge, 4: domain.Freezer, 5: domain.Freezer, 6: domain.Freezer,
	}

	want := []domain.PortionGroup{
		{Name: "lasagne", Nutrition: values, Location: domain.Fridge, Portions: 1},
		{Name: "chili", Nutrition: values, Location: domain.Freezer, Portions: 1},
		{Name: "Lasagne", Nutrition: values, Location: domain.Freezer, Portions: 2},
		{Name: "lasagne", Nutrition: lighter, Location: domain.Freezer, Portions: 1},
		{Name: "lasagne", Nutrition: knownFibre, Location: domain.Freezer, Portions: 1},
	}
	if got := domain.GroupPortions(portions, where); !reflect.DeepEqual(got, want) {
		t.Errorf("GroupPortions =\n%+v\nwant\n%+v", got, want)
	}
}

// One Movement adds all the Portions, which are where it put them from when it happened,
// and it leaves every Stock Item alone.
func TestAddingPortionsIsOneMovement(t *testing.T) {
	added := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	m := domain.NewLedger(nil).AddPortions(added, domain.Freezer, []int64{1, 2, 3}, "from Mom")
	if m.Cause != domain.Added || m.Change.Number() != 3 || m.Note != "from Mom" {
		t.Errorf("the Movement is %+v", m)
	}

	l := domain.NewLedger([]domain.Movement{m})
	want := map[int64]domain.StorageLocation{1: domain.Freezer, 2: domain.Freezer, 3: domain.Freezer}
	if got := l.PortionsAt(added); !reflect.DeepEqual(got, want) {
		t.Errorf("at the moment added, the Portions are at %v", got)
	}
	if got := l.PortionsAt(added.Add(-time.Second)); len(got) != 0 {
		t.Errorf("before they were added, the Portions are at %v", got)
	}
	if got := l.At(added); len(got) != 0 {
		t.Errorf("adding Portions touches the Stock Items: %v", got)
	}
}
