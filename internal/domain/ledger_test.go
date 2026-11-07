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

func onMonday(hour int) time.Time { return time.Date(2026, time.November, 2, hour, 0, 0, 0, time.UTC) }

var (
	riceFridge  = domain.StockKey{Ingredient: 1, Location: domain.Fridge}
	riceFreezer = domain.StockKey{Ingredient: 1, Location: domain.Freezer}
	ricePantry  = domain.StockKey{Ingredient: 1, Location: domain.Pantry}
	eggFridge   = domain.StockKey{Ingredient: 2, Location: domain.Fridge}
)

// counted is a Count at hour of key, which believed it held believed and was told stated.
func counted(hour int, key domain.StockKey, believed, stated domain.Quantity) domain.Movement {
	return domain.Movement{At: onMonday(hour), Key: key, Change: stated - believed, Cause: domain.Counted, Believed: believed, Stated: stated}
}

// moved is a move at hour of moved from key to another Storage Location.
func moved(hour int, key domain.StockKey, to domain.StorageLocation, moved domain.Quantity) domain.Movement {
	return domain.Movement{At: onMonday(hour), Key: key, To: to, Change: moved, Cause: domain.Moved}
}

// lastLinesAgree checks that each Stock Item's Movements end on what the Ledger holds of
// it once all of its Movements have happened.
func lastLinesAgree(t *testing.T, movements []domain.Movement) {
	t.Helper()
	l := domain.NewLedger(movements)
	end := onMonday(23)
	held := l.At(end)
	for _, m := range movements {
		keys := []domain.StockKey{m.Key}
		if m.Cause == domain.Moved {
			keys = append(keys, domain.StockKey{Ingredient: m.Key.Ingredient, Location: m.To})
		}
		for _, key := range keys {
			touched := l.Movements(key)
			if len(touched) == 0 {
				t.Fatalf("%v has no Movements, though one touched it", key)
			}
			if last := touched[len(touched)-1].After; last != held[key].Held {
				t.Errorf("%v's Movements end on %d, but the Ledger holds %d", key, last, held[key].Held)
			}
		}
	}
}

func TestTheLedgerAddsUpToAMoment(t *testing.T) {
	t.Run("nothing has happened", func(t *testing.T) {
		if got := domain.NewLedger(nil).At(onMonday(12)); len(got) != 0 {
			t.Errorf("a Ledger of no Movements holds %v, want nothing", got)
		}
	})

	movements := []domain.Movement{
		counted(9, ricePantry, 0, 7000),
		counted(9, eggFridge, 0, 60),
		counted(10, riceFreezer, 0, 2000),
		counted(11, ricePantry, 7000, 4500),
		counted(12, eggFridge, 60, 0),
	}
	cases := []struct {
		name string
		at   time.Time
		key  domain.StockKey
		want domain.Quantity
	}{
		{"before the first", onMonday(8), ricePantry, 0},
		{"at the moment of a Movement, which counts", onMonday(9), ricePantry, 7000},
		{"between Movements", onMonday(10), ricePantry, 7000},
		{"after a later one", onMonday(11), ricePantry, 4500},
		{"each Storage Location is its own Stock Item", onMonday(12), riceFreezer, 2000},
		{"counted to nothing", onMonday(12), eggFridge, 0},
	}
	l := domain.NewLedger(movements)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := l.At(c.at)[c.key].Held; got != c.want {
				t.Errorf("at %s %v holds %d, want %d", c.at.Format("15:04"), c.key, got, c.want)
			}
		})
	}
	lastLinesAgree(t, movements)

	t.Run("a Stock Item keeps when it was last counted", func(t *testing.T) {
		if got := l.At(onMonday(10))[ricePantry].Counted; !got.Equal(onMonday(9)) {
			t.Errorf("rice at 10:00 was last counted at %s, want 09:00", got.Format("15:04"))
		}
		if got := l.At(onMonday(12))[ricePantry].Counted; !got.Equal(onMonday(11)) {
			t.Errorf("rice at 12:00 was last counted at %s, want 11:00", got.Format("15:04"))
		}
	})

	t.Run("what each Storage Location holds of one Ingredient", func(t *testing.T) {
		got := l.HeldAt(onMonday(12), 1)
		want := map[domain.StorageLocation]domain.Quantity{domain.Pantry: 4500, domain.Freezer: 2000}
		if len(got) != len(want) || got[domain.Pantry] != want[domain.Pantry] || got[domain.Freezer] != want[domain.Freezer] {
			t.Errorf("rice at 12:00 is held %v, want %v", got, want)
		}
	})
}

// A move is one Movement that takes its quantity from one Stock Item and gives it to
// another of the same Ingredient, at one moment.
func TestAMoveTakesFromOneStockItemAndGivesToAnother(t *testing.T) {
	other := domain.StockKey{Ingredient: 2, Location: domain.Freezer}
	movements := []domain.Movement{
		counted(9, riceFridge, 0, 8000),
		counted(9, other, 0, 500),
		moved(10, riceFridge, domain.Freezer, 4000),
		moved(11, riceFridge, domain.Freezer, 4000),
	}
	cases := []struct {
		name string
		at   time.Time
		key  domain.StockKey
		want domain.Quantity
	}{
		{"not before it happens", onMonday(9), riceFridge, 8000},
		{"the source gives part of it", onMonday(10), riceFridge, 4000},
		{"the target takes it", onMonday(10), riceFreezer, 4000},
		{"all of it leaves the source at nothing", onMonday(11), riceFridge, 0},
		{"the target adds up what came", onMonday(11), riceFreezer, 8000},
		{"another Ingredient there is untouched", onMonday(11), other, 500},
	}
	l := domain.NewLedger(movements)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := l.At(c.at)[c.key].Held; got != c.want {
				t.Errorf("at %s %v holds %d, want %d", c.at.Format("15:04"), c.key, got, c.want)
			}
		})
	}
	lastLinesAgree(t, movements)

	t.Run("a move is not a Count of either Stock Item", func(t *testing.T) {
		items := l.At(onMonday(11))
		if got := items[riceFridge].Counted; !got.Equal(onMonday(9)) {
			t.Errorf("the source was last counted at %s, want 09:00", got.Format("15:04"))
		}
		if got := items[riceFreezer].Counted; !got.IsZero() {
			t.Errorf("the target was last counted at %s, want never", got.Format("15:04"))
		}
	})
}

// A Count stands: a Movement recorded after it but dated before it never changes what
// the kitchen held once it was counted, in what the Ledger holds or in a Stock Item's
// Movements. The Ledger has its Movements in the order they happened, so such a Movement
// comes first.
func TestACountStands(t *testing.T) {
	staple := domain.Movement{At: onMonday(12), Key: eggFridge, Change: -60, Cause: domain.MarkedStaple, Believed: 60}
	cases := []struct {
		name      string
		movements []domain.Movement
		key       domain.StockKey
		at        time.Time
		want      domain.Quantity
	}{
		{
			"a move recorded after a Count, dated before it",
			[]domain.Movement{counted(9, riceFridge, 0, 8000), moved(10, riceFridge, domain.Freezer, 3000), counted(11, riceFridge, 8000, 6000)},
			riceFridge, onMonday(11), 6000,
		},
		{
			"its target still takes what came",
			[]domain.Movement{counted(9, riceFridge, 0, 8000), moved(10, riceFridge, domain.Freezer, 3000), counted(11, riceFridge, 8000, 6000)},
			riceFreezer, onMonday(11), 3000,
		},
		{
			"what follows the Count adds to what was stated",
			[]domain.Movement{counted(9, riceFridge, 0, 8000), moved(10, riceFridge, domain.Freezer, 3000), counted(11, riceFridge, 8000, 6000), moved(12, riceFridge, domain.Freezer, 1000)},
			riceFridge, onMonday(12), 5000,
		},
		{
			"a Staple's Count to nothing stands too",
			[]domain.Movement{counted(9, eggFridge, 0, 60), moved(11, eggFridge, domain.Freezer, 20), staple},
			eggFridge, onMonday(12), 0,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := domain.NewLedger(c.movements).At(c.at)[c.key].Held; got != c.want {
				t.Errorf("at %s %v holds %d, want %d", c.at.Format("15:04"), c.key, got, c.want)
			}
			lastLinesAgree(t, c.movements)
		})
	}

	t.Run("a Staple's Count to nothing doesn't set when it was last counted", func(t *testing.T) {
		got := domain.NewLedger([]domain.Movement{counted(9, eggFridge, 0, 60), staple}).At(onMonday(12))[eggFridge].Counted
		if !got.Equal(onMonday(9)) {
			t.Errorf("the egg was last counted at %s, want 09:00", got.Format("15:04"))
		}
	})
}

// A Stock Item's Movements are all that touched it, on either side of a move, each with
// what it held before and after. A Count shows what it believed and what was stated.
func TestAStockItemsMovements(t *testing.T) {
	movements := []domain.Movement{
		counted(8, riceFridge, 0, 8000),
		counted(8, eggFridge, 0, 60),
		counted(9, riceFreezer, 0, 1000),
		{At: onMonday(10), Key: riceFridge, To: domain.Freezer, Change: 4000, Cause: domain.Moved, Note: "for next week"},
		counted(11, riceFridge, 4000, 3500),
		moved(12, riceFreezer, domain.Pantry, 5000),
		counted(13, riceFridge, 3500, 3500),
	}
	type shown struct {
		at            int
		before, after domain.Quantity
	}
	cases := []struct {
		name      string
		movements []domain.Movement
		key       domain.StockKey
		want      []shown
	}{
		{"the source of a move", movements, riceFridge, []shown{{8, 0, 8000}, {10, 8000, 4000}, {11, 4000, 3500}, {13, 3500, 3500}}},
		{"the target, then the source", movements, riceFreezer, []shown{{9, 0, 1000}, {10, 1000, 5000}, {12, 5000, 0}}},
		{"a target alone", movements, ricePantry, []shown{{12, 0, 5000}}},
		{"nothing touched it", movements, domain.StockKey{Ingredient: 3, Location: domain.Pantry}, nil},
		{
			"a Count after a Movement dated before it but recorded after it",
			[]domain.Movement{counted(9, riceFridge, 0, 8000), moved(10, riceFridge, domain.Freezer, 3000), counted(11, riceFridge, 8000, 6000)},
			riceFridge,
			[]shown{{9, 0, 8000}, {10, 8000, 5000}, {11, 8000, 6000}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := domain.NewLedger(c.movements).Movements(c.key)
			if len(got) != len(c.want) {
				t.Fatalf("the Stock Item has %d Movements, want %d: %+v", len(got), len(c.want), got)
			}
			for i, want := range c.want {
				if !got[i].At.Equal(onMonday(want.at)) || got[i].Before != want.before || got[i].After != want.after {
					t.Errorf("Movement %d = %s %d → %d, want %d:00 %d → %d", i, got[i].At.Format("15:04"), got[i].Before, got[i].After, want.at, want.before, want.after)
				}
			}
			if len(got) > 0 {
				lastLinesAgree(t, c.movements)
			}
		})
	}

	t.Run("each is its Movement, note and all", func(t *testing.T) {
		if got := domain.NewLedger(movements).Movements(riceFreezer)[1]; got.Note != "for next week" || got.To != domain.Freezer {
			t.Errorf("the move in the freezer's Movements is %+v", got)
		}
	})
}

// A Movement written while the kitchen is held happens at the later of now and the last
// Movement, so it is checked against, and comes after, all that the Ledger holds.
func TestTheLedgerWritesAMovement(t *testing.T) {
	ahead := []domain.Movement{counted(9, riceFridge, 0, 8000), counted(11, riceFridge, 8000, 6000)}
	behind := []domain.Movement{counted(9, riceFridge, 0, 8000)}

	t.Run("a Count", func(t *testing.T) {
		cases := []struct {
			name      string
			movements []domain.Movement
			want      domain.Movement
		}{
			{"of a Stock Item nothing has reached", nil, counted(10, riceFridge, 0, 3000)},
			{"from what is held now", behind, counted(10, riceFridge, 8000, 3000)},
			{"after a later Movement, from what it left", ahead, counted(11, riceFridge, 6000, 3000)},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got := domain.NewLedger(c.movements).Count(onMonday(10), riceFridge, 3000, "")
				if got != c.want {
					t.Errorf("the Count is %+v, want %+v", got, c.want)
				}
				lastLinesAgree(t, append(c.movements, got))
			})
		}
	})

	t.Run("a Count keeps its note", func(t *testing.T) {
		if got := domain.NewLedger(nil).Count(onMonday(10), riceFridge, 3000, "the top shelf"); got.Note != "the top shelf" {
			t.Errorf("the Count's note is %q", got.Note)
		}
	})

	t.Run("a move", func(t *testing.T) {
		cases := []struct {
			name      string
			movements []domain.Movement
			moved     domain.Quantity
			wantAt    time.Time
			problem   domain.Problem
			held      domain.Quantity
		}{
			{"of what is held now", behind, 7000, onMonday(10), "", 8000},
			{"after a later Movement", ahead, 5000, onMonday(11), "", 6000},
			{"of more than a later Movement left", ahead, 7000, onMonday(11), domain.MoreThanHeld, 6000},
			{"of nothing", behind, 0, onMonday(10), domain.NothingMoved, 8000},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				got, problem, held := domain.NewLedger(c.movements).Move(onMonday(10), riceFridge, domain.Freezer, c.moved, "")
				if problem != c.problem || held != c.held {
					t.Fatalf("the move is refused %q from %d held, want %q from %d", problem, held, c.problem, c.held)
				}
				if problem != "" {
					return
				}
				if want := (domain.Movement{At: c.wantAt, Key: riceFridge, To: domain.Freezer, Change: c.moved, Cause: domain.Moved}); got != want {
					t.Errorf("the move is %+v, want %+v", got, want)
				}
				lastLinesAgree(t, append(c.movements, got))
			})
		}
	})

	t.Run("a Staple's Counts to nothing", func(t *testing.T) {
		movements := []domain.Movement{
			counted(9, riceFridge, 0, 8000),
			counted(9, riceFreezer, 0, 2000),
			counted(9, eggFridge, 0, 60),
			moved(11, riceFreezer, domain.Pantry, 2000),
		}
		got := domain.NewLedger(movements).CountToNothing(onMonday(10), 1)
		want := []domain.Movement{
			{At: onMonday(11), Key: ricePantry, Change: -2000, Cause: domain.MarkedStaple, Believed: 2000},
			{At: onMonday(11), Key: riceFridge, Change: -8000, Cause: domain.MarkedStaple, Believed: 8000},
		}
		if len(got) != len(want) {
			t.Fatalf("the Counts to nothing are %+v, want %+v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("Count to nothing %d is %+v, want %+v", i, got[i], want[i])
			}
		}
		lastLinesAgree(t, append(movements, got...))
	})
}
