package domain_test

import (
	"testing"
	"time"

	"github.com/simopzz/dandori/internal/domain"
)

// Grams and millilitres are whole numbers, pieces have one decimal place, either decimal
// mark will do, and nothing is negative.
func TestParseQuantity(t *testing.T) {
	cases := []struct {
		name    string
		unit    domain.Unit
		typed   string
		want    domain.Quantity
		problem domain.Problem
	}{
		{"whole grams", domain.Grams, "700", 7000, ""},
		{"whole millilitres", domain.Millilitres, " 250 ", 2500, ""},
		{"nothing", domain.Grams, "0", 0, ""},
		{"half a piece", domain.Pieces, "0.5", 5, ""},
		{"a decimal comma", domain.Pieces, "1,5", 15, ""},
		{"whole pieces", domain.Pieces, "3", 30, ""},
		{"a tenth a float can't hold", domain.Pieces, "0.3", 3, ""},
		{"a point zero is whole", domain.Grams, "700.0", 7000, ""},
		{"the largest", domain.Grams, "999999", 9999990, ""},
		{"blank", domain.Grams, "  ", 0, domain.Required},
		{"not a number", domain.Grams, "lots", 0, domain.NotANumber},
		{"not finite", domain.Grams, "Inf", 0, domain.NotANumber},
		{"below nothing", domain.Grams, "-1", 0, domain.Negative},
		{"pieces below nothing", domain.Pieces, "-0.5", 0, domain.Negative},
		{"part of a gram", domain.Grams, "12.5", 0, domain.NotWhole},
		{"part of a millilitre", domain.Millilitres, "0,5", 0, domain.NotWhole},
		{"two decimal places of a piece", domain.Pieces, "0.25", 0, domain.TooPrecise},
		{"too much", domain.Grams, "1000000", 0, domain.TooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, problem := domain.ParseQuantity(c.unit, c.typed)
			if got != c.want || problem != c.problem {
				t.Errorf("ParseQuantity(%s, %q) = %d, %q; want %d, %q", c.unit, c.typed, got, problem, c.want, c.problem)
			}
		})
	}
}

// A quantity reads back as it was typed, with no decimal place unless it has one.
func TestAQuantityIsANumber(t *testing.T) {
	cases := []struct {
		q    domain.Quantity
		want float64
	}{{0, 0}, {5, 0.5}, {15, 1.5}, {7000, 700}}
	for _, c := range cases {
		if got := c.q.Number(); got != c.want {
			t.Errorf("Quantity(%d).Number() = %v, want %v", c.q, got, c.want)
		}
	}
}

// A Count's difference is what the User stated less what Dandori believed.
func TestACountsDifference(t *testing.T) {
	cases := []struct {
		name             string
		believed, stated domain.Quantity
		want             domain.Quantity
	}{
		{"from nothing", 0, 7000, 7000},
		{"up", 2000, 7000, 5000},
		{"down", 7000, 4500, -2500},
		{"to nothing", 7000, 0, -7000},
		{"the same", 7000, 7000, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.CountDifference(c.believed, c.stated); got != c.want {
				t.Errorf("CountDifference(%d, %d) = %d, want %d", c.believed, c.stated, got, c.want)
			}
		})
	}
}

func TestTheLedgerAddsUpToAMoment(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, time.November, 2, hour, 0, 0, 0, time.UTC) }
	rice := domain.StockKey{Ingredient: 1, Location: domain.Pantry}
	riceFrozen := domain.StockKey{Ingredient: 1, Location: domain.Freezer}
	egg := domain.StockKey{Ingredient: 2, Location: domain.Fridge}
	count := func(hour int, key domain.StockKey, change domain.Quantity) domain.Movement {
		return domain.Movement{At: at(hour), Key: key, Change: change, Cause: domain.Counted}
	}

	t.Run("nothing has happened", func(t *testing.T) {
		if got := domain.AddUp(nil, at(12)); len(got) != 0 {
			t.Errorf("AddUp of no Movements = %v, want nothing", got)
		}
	})

	movements := []domain.Movement{
		count(9, rice, 7000),
		count(9, egg, 60),
		count(10, riceFrozen, 2000),
		count(11, rice, -2500),
		count(12, egg, -60),
	}
	cases := []struct {
		name string
		at   time.Time
		key  domain.StockKey
		want domain.Quantity
	}{
		{"before the first", at(8), rice, 0},
		{"at the moment of a Movement, which counts", at(9), rice, 7000},
		{"between Movements", at(10), rice, 7000},
		{"after a later one", at(11), rice, 4500},
		{"each Storage Location is its own Stock Item", at(12), riceFrozen, 2000},
		{"counted to nothing", at(12), egg, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.AddUp(movements, c.at)[c.key].Held; got != c.want {
				t.Errorf("at %s %v holds %d, want %d", c.at.Format("15:04"), c.key, got, c.want)
			}
		})
	}

	t.Run("a Stock Item keeps when it was last counted", func(t *testing.T) {
		if got := domain.AddUp(movements, at(10))[rice].Counted; !got.Equal(at(9)) {
			t.Errorf("rice at 10:00 was last counted at %s, want 09:00", got.Format("15:04"))
		}
		if got := domain.AddUp(movements, at(12))[rice].Counted; !got.Equal(at(11)) {
			t.Errorf("rice at 12:00 was last counted at %s, want 11:00", got.Format("15:04"))
		}
	})
}

// A move is one Movement that takes its quantity from one Stock Item and gives it to
// another of the same Ingredient, at one moment.
func TestAMoveTakesFromOneStockItemAndGivesToAnother(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, time.November, 2, hour, 0, 0, 0, time.UTC) }
	fridge := domain.StockKey{Ingredient: 1, Location: domain.Fridge}
	freezer := domain.StockKey{Ingredient: 1, Location: domain.Freezer}
	other := domain.StockKey{Ingredient: 2, Location: domain.Freezer}
	movements := []domain.Movement{
		{At: at(9), Key: fridge, Change: 8000, Cause: domain.Counted},
		{At: at(9), Key: other, Change: 500, Cause: domain.Counted},
		{At: at(10), Key: fridge, To: domain.Freezer, Change: 4000, Cause: domain.Moved},
		{At: at(11), Key: fridge, To: domain.Freezer, Change: 4000, Cause: domain.Moved},
	}
	cases := []struct {
		name string
		at   time.Time
		key  domain.StockKey
		want domain.Quantity
	}{
		{"not before it happens", at(9), fridge, 8000},
		{"the source gives part of it", at(10), fridge, 4000},
		{"the target takes it", at(10), freezer, 4000},
		{"all of it leaves the source at nothing", at(11), fridge, 0},
		{"the target adds up what came", at(11), freezer, 8000},
		{"another Ingredient there is untouched", at(11), other, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.AddUp(movements, c.at)[c.key].Held; got != c.want {
				t.Errorf("at %s %v holds %d, want %d", c.at.Format("15:04"), c.key, got, c.want)
			}
		})
	}

	t.Run("a move is not a Count of either Stock Item", func(t *testing.T) {
		items := domain.AddUp(movements, at(11))
		if got := items[fridge].Counted; !got.Equal(at(9)) {
			t.Errorf("the source was last counted at %s, want 09:00", got.Format("15:04"))
		}
		if got := items[freezer].Counted; !got.IsZero() {
			t.Errorf("the target was last counted at %s, want never", got.Format("15:04"))
		}
	})
}

// A move can take no more than is held, and must take something.
func TestCheckMove(t *testing.T) {
	cases := []struct {
		name        string
		held, moved domain.Quantity
		want        domain.Problem
	}{
		{"part of it", 8000, 4000, ""},
		{"all of it", 8000, 8000, ""},
		{"a tenth of a piece", 1, 1, ""},
		{"nothing", 8000, 0, domain.NothingMoved},
		{"a tenth too much", 8000, 8001, domain.MoreThanHeld},
		{"from nothing held", 0, 10, domain.MoreThanHeld},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.CheckMove(c.held, c.moved); got != c.want {
				t.Errorf("CheckMove(%d, %d) = %q, want %q", c.held, c.moved, got, c.want)
			}
		})
	}
}

// A Stock Item's history is every Movement that touched it, on either side of a move,
// each with what it held before and after. A Count shows what it kept.
func TestAStockItemsHistory(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, time.November, 2, hour, 0, 0, 0, time.UTC) }
	fridge := domain.StockKey{Ingredient: 1, Location: domain.Fridge}
	freezer := domain.StockKey{Ingredient: 1, Location: domain.Freezer}
	pantry := domain.StockKey{Ingredient: 1, Location: domain.Pantry}
	movements := []domain.Movement{
		{At: at(8), Key: fridge, Change: 8000, Cause: domain.Counted},
		{At: at(8), Key: domain.StockKey{Ingredient: 2, Location: domain.Fridge}, Change: 60, Cause: domain.Counted},
		{At: at(9), Key: freezer, Change: 1000, Cause: domain.Counted},
		{At: at(10), Key: fridge, To: domain.Freezer, Change: 4000, Cause: domain.Moved, Note: "for next week"},
		{At: at(11), Key: fridge, Believed: 4000, Change: -500, Cause: domain.Counted},
		{At: at(12), Key: freezer, To: domain.Pantry, Change: 5000, Cause: domain.Moved},
		{At: at(13), Key: fridge, Believed: 3500, Change: 0, Cause: domain.Counted},
	}
	type step struct {
		at            int
		before, after domain.Quantity
	}
	cases := []struct {
		name string
		key  domain.StockKey
		want []step
	}{
		{"the source of a move", fridge, []step{{8, 0, 8000}, {10, 8000, 4000}, {11, 4000, 3500}, {13, 3500, 3500}}},
		{"the target, then the source", freezer, []step{{9, 0, 1000}, {10, 1000, 5000}, {12, 5000, 0}}},
		{"a target alone", pantry, []step{{12, 0, 5000}}},
		{"nothing touched it", domain.StockKey{Ingredient: 3, Location: domain.Pantry}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := domain.History(movements, c.key)
			if len(got) != len(c.want) {
				t.Fatalf("History has %d steps, want %d: %+v", len(got), len(c.want), got)
			}
			for i, want := range c.want {
				if !got[i].At.Equal(at(want.at)) || got[i].Before != want.before || got[i].After != want.after {
					t.Errorf("step %d = %s %d → %d, want %d:00 %d → %d", i, got[i].At.Format("15:04"), got[i].Before, got[i].After, want.at, want.before, want.after)
				}
			}
		})
	}

	t.Run("a step is its Movement, note and all", func(t *testing.T) {
		if got := domain.History(movements, freezer)[1]; got.Note != "for next week" || got.To != domain.Freezer {
			t.Errorf("the move in the freezer's history is %+v", got)
		}
	})

	// One Count that overlapped another may be dated before the Movement it read, so a
	// running balance alone would be out by what that one changed.
	t.Run("a Count shows what it believed, not a running balance", func(t *testing.T) {
		overlapped := []domain.Movement{
			{At: at(9), Key: fridge, Believed: 3000, Change: 1000, Cause: domain.Counted},
			{At: at(10), Key: fridge, Believed: 0, Change: 3000, Cause: domain.Counted},
		}
		got := domain.History(overlapped, fridge)[0]
		if got.Before != 3000 || got.After != 4000 {
			t.Errorf("the Count shows %d → %d, want what it kept, 3000 → 4000", got.Before, got.After)
		}
	})
}
