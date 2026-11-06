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
