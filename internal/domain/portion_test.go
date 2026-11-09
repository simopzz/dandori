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
		{Name: "lasagne", Nutrition: values, Location: domain.Fridge, IDs: []int64{3}},
		{Name: "chili", Nutrition: values, Location: domain.Freezer, IDs: []int64{6}},
		{Name: "Lasagne", Nutrition: values, Location: domain.Freezer, IDs: []int64{1, 2}},
		{Name: "lasagne", Nutrition: lighter, Location: domain.Freezer, IDs: []int64{4}},
		{Name: "lasagne", Nutrition: knownFibre, Location: domain.Freezer, IDs: []int64{5}},
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

func TestParsePortions(t *testing.T) {
	values := map[string]string{"kcal": "620", "protein": "32", "carbs": "58", "fat": "28", "fibre": "", "sugar": "8"}
	want := domain.NewPortions{
		Name: "Lasagne", HowMany: 3, Location: domain.Freezer, Note: "from Sunday",
		Nutrition: domain.Nutrition{Kcal: domain.Known(620), Protein: domain.Known(32), Carbs: domain.Known(58), Fat: domain.Known(28), Sugar: domain.Known(8)},
	}

	got, problems := domain.ParsePortions(domain.TypedPortions{
		Name: " Lasagne ", HowMany: "3", Location: domain.Freezer, Nutrition: values, Note: " from Sunday ",
	})
	if len(problems) != 0 || !reflect.DeepEqual(got, want) {
		t.Errorf("ParsePortions = %+v, %v; want %+v and no problems", got, problems, want)
	}

	// Everything refused is refused at once, an optional value only if it is garbled.
	_, problems = domain.ParsePortions(domain.TypedPortions{
		Name: "  ", HowMany: "100", Location: "garage",
		Nutrition: map[string]string{"kcal": "", "protein": "x", "carbs": "1", "fat": "1", "fibre": "-1", "sugar": ""},
	})
	wantProblems := map[string]domain.Problem{
		"name": domain.Required, "how_many": domain.TooLarge, "location": domain.Required,
		"kcal": domain.Required, "protein": domain.NotANumber, "fibre": domain.Negative,
	}
	if !reflect.DeepEqual(problems, wantProblems) {
		t.Errorf("problems = %v, want %v", problems, wantProblems)
	}
}

// A moved Portion is where it went, and one a Count took is no longer held, from the
// moment each happened.
func TestPortionsFollowTheirMovements(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, 11, 2, hour, 0, 0, 0, time.UTC) }
	l := domain.NewLedger([]domain.Movement{
		{At: at(10), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{1, 2, 3}, Change: domain.QuantityOf(3), Cause: domain.Added},
		{At: at(11), Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Portions: []int64{1}, Change: domain.QuantityOf(1), Cause: domain.Moved},
		{At: at(12), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{2}, Change: domain.QuantityOf(-1), Cause: domain.Counted, Believed: domain.QuantityOf(2), Stated: domain.QuantityOf(1)},
		{At: at(13), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{4}, Change: domain.QuantityOf(1), Cause: domain.Counted, Believed: domain.QuantityOf(1), Stated: domain.QuantityOf(2)},
		{At: at(14), Key: domain.StockKey{Location: domain.Freezer}, Portions: []int64{3, 4}, Cause: domain.Counted, Believed: domain.QuantityOf(2), Stated: domain.QuantityOf(2)},
	})
	cases := []struct {
		hour int
		want map[int64]domain.StorageLocation
	}{
		{10, map[int64]domain.StorageLocation{1: domain.Freezer, 2: domain.Freezer, 3: domain.Freezer}},
		{11, map[int64]domain.StorageLocation{1: domain.Fridge, 2: domain.Freezer, 3: domain.Freezer}},
		{12, map[int64]domain.StorageLocation{1: domain.Fridge, 3: domain.Freezer}},
		{13, map[int64]domain.StorageLocation{1: domain.Fridge, 3: domain.Freezer, 4: domain.Freezer}},
		{14, map[int64]domain.StorageLocation{1: domain.Fridge, 3: domain.Freezer, 4: domain.Freezer}},
	}
	for _, c := range cases {
		if got := l.PortionsAt(at(c.hour)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("at %d:00 the Portions are at %v, want %v", c.hour, got, c.want)
		}
	}
}

// whole is n Portions as a Quantity, as a Movement holds them.
func whole(n float64) domain.Quantity { return domain.QuantityOf(n) }

// A group is what is alike the Portion asked for, in that Storage Location; one no longer
// held still names it, with nothing in it, and an unknown Portion has no group.
func TestGroupOf(t *testing.T) {
	values := domain.Nutrition{Kcal: domain.Known(620), Protein: domain.Known(32), Carbs: domain.Known(58), Fat: domain.Known(28)}
	lighter := values
	lighter.Kcal = domain.Known(480)
	portions := []domain.Portion{
		{ID: 1, Name: "Lasagne", Nutrition: values},
		{ID: 2, Name: "lasagne", Nutrition: values},
		{ID: 3, Name: "LASAGNE", Nutrition: values},
		{ID: 4, Name: "lasagne", Nutrition: lighter},
	}
	where := map[int64]domain.StorageLocation{2: domain.Freezer, 3: domain.Freezer, 4: domain.Freezer}

	got, ok := domain.GroupOf(portions, where, 3, domain.Freezer)
	want := domain.PortionGroup{Name: "lasagne", Nutrition: values, Location: domain.Freezer, IDs: []int64{2, 3}}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("GroupOf = %+v, %v; want %+v", got, ok, want)
	}
	got, ok = domain.GroupOf(portions, where, 1, domain.Fridge)
	want = domain.PortionGroup{Name: "Lasagne", Nutrition: values, Location: domain.Fridge}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("GroupOf a place holding none = %+v, %v; want %+v", got, ok, want)
	}
	if _, ok := domain.GroupOf(portions, where, 9, domain.Freezer); ok {
		t.Error("an unknown Portion has a group")
	}
}

func TestParsePortionCount(t *testing.T) {
	cases := []struct {
		typed   string
		want    float64
		problem domain.Problem
	}{
		{"0", 0, ""},
		{" 99 ", 99, ""},
		{"", 0, domain.Required},
		{"-1", 0, domain.Negative},
		{"1.5", 0, domain.NotWhole},
		{"a few", 0, domain.NotANumber},
		{"100", 0, domain.TooLarge},
	}
	for _, c := range cases {
		got, problems := domain.ParsePortionCount(domain.Freezer, c.typed)
		if c.problem != problems["stated"] || (c.problem == "" && got != whole(c.want)) {
			t.Errorf("ParsePortionCount(%q) = %v, %v; want %v, %q", c.typed, got, problems, c.want, c.problem)
		}
	}
	if _, problems := domain.ParsePortionCount("garage", "1"); problems["location"] != domain.Required {
		t.Errorf("a place that isn't a Storage Location: %v", problems)
	}
}

// A Count keeps what was believed and stated. Counting down takes those without a Slot,
// oldest first, then the latest Slot; counting up lists what was added; and a Count
// that changes nothing lists the whole group, so its Movement can be told as the group's.
func TestCountingPortions(t *testing.T) {
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	tuesday, friday := now.AddDate(0, 0, 1), now.AddDate(0, 0, 4)
	group := domain.PortionGroup{Name: "lasagne", Location: domain.Freezer, IDs: []int64{1, 2, 3, 4}}
	slots := map[int64]time.Time{2: tuesday, 3: friday}

	cases := []struct {
		name         string
		stated       float64
		added        []int64
		slots        map[int64]time.Time
		wantChange   float64
		wantPortions []int64
	}{
		{"down, oldest first without Slots", 2, nil, nil, -2, []int64{1, 2}},
		{"down, those without a Slot first", 2, nil, slots, -2, []int64{1, 4}},
		{"down, then the latest Slot", 1, nil, slots, -3, []int64{1, 4, 3}},
		{"to nothing", 0, nil, slots, -4, []int64{1, 4, 3, 2}},
		{"up, the Portions added", 6, []int64{7, 8}, nil, 2, []int64{7, 8}},
		{"no change, the whole group", 4, nil, slots, 0, []int64{1, 2, 3, 4}},
	}
	for _, c := range cases {
		m := domain.NewLedger(nil).CountPortions(now, group, whole(c.stated), c.added, c.slots, "note")
		if m.Cause != domain.Counted || m.Change != whole(c.wantChange) || m.Believed != whole(4) || m.Stated != whole(c.stated) ||
			m.Key.Location != domain.Freezer || m.Note != "note" || !m.At.Equal(now) {
			t.Errorf("%s: the Movement is %+v", c.name, m)
		}
		if !reflect.DeepEqual(m.Portions, c.wantPortions) {
			t.Errorf("%s: it lists %v, want %v", c.name, m.Portions, c.wantPortions)
		}
	}
}

// Counting up adds more alike the group, spelled as it is.
func TestCountingUpAddsMoreAlike(t *testing.T) {
	values := domain.Nutrition{Kcal: domain.Known(620)}
	group := domain.PortionGroup{Name: "Lasagne", Nutrition: values, Location: domain.Freezer, IDs: []int64{1}}
	want := domain.NewPortions{Name: "Lasagne", HowMany: 2, Location: domain.Freezer, Nutrition: values}
	if got := group.MoreAlike(whole(3)); !reflect.DeepEqual(got, want) {
		t.Errorf("MoreAlike = %+v, want %+v", got, want)
	}
}

// A move takes the oldest of the group, at least one and no more than it holds.
func TestMovingPortions(t *testing.T) {
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	group := domain.PortionGroup{Name: "lasagne", Location: domain.Freezer, IDs: []int64{1, 2, 3}}

	m, problem := domain.NewLedger(nil).MovePortions(now, group, domain.Fridge, whole(2), "to thaw")
	want := domain.Movement{
		At: now, Key: domain.StockKey{Location: domain.Freezer}, To: domain.Fridge, Change: whole(2),
		Cause: domain.Moved, Portions: []int64{1, 2}, Note: "to thaw",
	}
	if problem != "" || !reflect.DeepEqual(m, want) {
		t.Errorf("MovePortions = %+v, %q; want %+v", m, problem, want)
	}

	for moved, want := range map[float64]domain.Problem{0: domain.NothingMoved, 4: domain.MoreThanHeld, 3: ""} {
		if _, problem := domain.NewLedger(nil).MovePortions(now, group, domain.Fridge, whole(moved), ""); problem != want {
			t.Errorf("moving %v of 3 is %q, want %q", moved, problem, want)
		}
	}
}

// A group's Movements begin when the oldest Portion it holds now arrived there, leaving
// out an earlier life of the group, and each tells what it held before and after it.
func TestGroupMovements(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, 11, 2, hour, 0, 0, 0, time.UTC) }
	freezer := domain.StockKey{Location: domain.Freezer}
	portions := []domain.Portion{{ID: 1, Name: "lasagne"}, {ID: 2, Name: "lasagne"}, {ID: 3, Name: "lasagne"}, {ID: 4, Name: "lasagne"}, {ID: 9, Name: "chili"}}
	l := domain.NewLedger([]domain.Movement{
		{At: at(8), Key: freezer, Portions: []int64{1}, Change: whole(1), Cause: domain.Added},
		{At: at(9), Key: freezer, Portions: []int64{1}, Change: whole(-1), Cause: domain.Counted, Believed: whole(1), Stated: whole(0)},
		{At: at(10), Key: freezer, Portions: []int64{2, 3, 4}, Change: whole(3), Cause: domain.Added},
		{At: at(10), Key: freezer, Portions: []int64{9}, Change: whole(1), Cause: domain.Added},
		{At: at(11), Key: freezer, Portions: []int64{2}, Change: whole(-1), Cause: domain.Counted, Believed: whole(3), Stated: whole(2)},
		{At: at(12), Key: freezer, To: domain.Fridge, Portions: []int64{3}, Change: whole(1), Cause: domain.Moved},
	})

	type row struct {
		hour          int
		before, after float64
	}
	check := func(location domain.StorageLocation, id int64, want []row) {
		t.Helper()
		var got []row
		for _, m := range l.GroupMovements(portions, id, location) {
			got = append(got, row{m.At.Hour(), m.Before.Number(), m.After.Number()})
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: %+v, want %+v", location, got, want)
		}
	}
	check(domain.Freezer, 4, []row{{10, 0, 3}, {11, 3, 2}, {12, 2, 1}})
	check(domain.Fridge, 3, []row{{12, 0, 1}})
	check(domain.Pantry, 4, nil)
}
