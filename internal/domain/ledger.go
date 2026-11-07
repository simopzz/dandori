package domain

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Quantity is how much of an Ingredient, in its Unit, as a whole number of tenths: what
// a Stock Item holds, or a Movement's change, which may be below nothing. Tenths add up
// exactly, as the floats a piece's decimal place would be don't.
type Quantity int64

// tenths is how many of a Quantity make one of its Unit.
const tenths = 10

// maxQuantity bounds a quantity the User states: no kitchen holds a tonne of anything.
const maxQuantity = 1000000

// Number is q in its Unit.
func (q Quantity) Number() float64 { return float64(q) / tenths }

// QuantityOf is the Quantity of v in its Unit, as Number gives it.
func QuantityOf(v float64) Quantity { return Quantity(math.Round(v * tenths)) }

// ParseQuantity reads a quantity of unit as the User typed it, with a point or a comma:
// grams and millilitres are whole numbers, pieces have one decimal place, and nothing
// is negative.
func ParseQuantity(unit Unit, typed string) (Quantity, Problem) {
	typed = strings.ReplaceAll(strings.TrimSpace(typed), ",", ".")
	if typed == "" {
		return 0, Required
	}
	v, err := strconv.ParseFloat(typed, 64)
	switch {
	case err != nil || math.IsNaN(v) || math.IsInf(v, 0):
		return 0, NotANumber
	case v < 0:
		return 0, Negative
	case v >= maxQuantity:
		return 0, TooLarge
	case unit != Pieces && v != math.Trunc(v):
		return 0, NotWhole
	case math.Abs(v*tenths-math.Round(v*tenths)) > 1e-9:
		return 0, TooPrecise
	}
	return QuantityOf(v), ""
}

// Cause is the kind of thing that caused a Movement. The Kitchen Ledger stores it as
// data, so a page can tell it in the User's Language.
type Cause string

// The Causes of a Movement.
const (
	// Counted is the Cause of a Count.
	Counted Cause = "counted"
	// Moved is the Cause of a move.
	Moved Cause = "moved"
	// MarkedStaple is the Cause of the Count to nothing that marking its Ingredient a
	// Staple makes of each Stock Item holding some of it.
	MarkedStaple Cause = "staple"
)

// sets says whether a Movement of cause c sets what its Stock Item holds to what was
// stated, keeping what Dandori believed before it, as a Count does.
func (c Cause) sets() bool { return c == Counted || c == MarkedStaple }

// Why a move is refused.
const (
	NothingMoved Problem = "nothing moved"
	MoreThanHeld Problem = "more than held"
)

// StockKey identifies a Stock Item: there is one per Ingredient and Storage Location.
type StockKey struct {
	Ingredient int64
	Location   StorageLocation
}

// Movement is one change to what the kitchen holds. A move is one Movement: Key is the
// Stock Item it comes from, To the Storage Location it goes to, and Change, above
// nothing, how much goes.
type Movement struct {
	At     time.Time
	Key    StockKey
	To     StorageLocation
	Change Quantity
	Cause  Cause
	// Believed is what a Count, or the Count to nothing a Staple makes, kept of what
	// Dandori believed before it, and Stated what it was told the Stock Item holds.
	Believed, Stated Quantity
	// Note is the User's, if any.
	Note string
}

// StockMovement is a Movement of one Stock Item with what it held before and after.
type StockMovement struct {
	Movement
	Before, After Quantity
}

// StockItem is what one Ingredient's Movements in one Storage Location add up to.
type StockItem struct {
	Held Quantity
	// Counted is when it was last counted; it is zero if it never was.
	Counted time.Time
}

// CheckMove says why a move of moved is refused from a Stock Item holding held, if it is.
func CheckMove(held, moved Quantity) Problem {
	switch {
	case moved <= 0:
		return NothingMoved
	case moved > held:
		return MoreThanHeld
	}
	return ""
}

// ParseCount reads a Count of unit as the User typed it: the Storage Location it is of
// and the quantity they state is there, and problems, why each field is refused, by
// field, if any is.
func ParseCount(unit Unit, location StorageLocation, stated string) (Quantity, map[string]Problem) {
	problems := map[string]Problem{}
	if !slices.Contains(StorageLocations, location) {
		problems["location"] = Required
	}
	q, problem := ParseQuantity(unit, stated)
	if problem != "" {
		problems["stated"] = problem
	}
	return q, problems
}

// ParseMove reads a move of unit from one Storage Location as the User typed it: the
// Storage Location it goes to, another, and the quantity to move, and problems, why each
// field is refused, by field, if any is.
func ParseMove(unit Unit, from, to StorageLocation, moved string) (Quantity, map[string]Problem) {
	problems := map[string]Problem{}
	if to == from || !slices.Contains(StorageLocations, to) {
		problems["to"] = Required
	}
	q, problem := ParseQuantity(unit, moved)
	if problem != "" {
		problems["quantity"] = problem
	}
	return q, problems
}

// after is what the Stock Item key holds after m, from held before it; touches is false
// if m leaves key alone. A move touches both of its Stock Items, and a Count stands: it
// sets its Stock Item to what was stated, whatever came before it.
func (m Movement) after(key StockKey, held Quantity) (_ Quantity, touches bool) {
	switch {
	case m.Key == key && m.Cause.sets():
		return m.Stated, true
	case m.Key == key && m.Cause == Moved:
		return held - m.Change, true
	case m.Key == key:
		return held + m.Change, true
	case m.Cause == Moved && m.Key.Ingredient == key.Ingredient && m.To == key.Location:
		return held + m.Change, true
	}
	return held, false
}

// Ledger is the Kitchen Ledger: its Movements in the order they happened, those of one
// moment in the order they were recorded. It adds them up one way, so what the kitchen
// holds at a moment and a Stock Item's Movements agree.
type Ledger struct{ movements []Movement }

// NewLedger is the Ledger of movements, in the order they happened.
func NewLedger(movements []Movement) Ledger { return Ledger{movements: movements} }

// At is what the kitchen holds at a moment, by Stock Item. One Movement at that very
// moment counts.
func (l Ledger) At(at time.Time) map[StockKey]StockItem {
	items := map[StockKey]StockItem{}
	for _, m := range l.movements {
		if m.At.After(at) {
			continue
		}
		keys := []StockKey{m.Key}
		if m.Cause == Moved {
			keys = append(keys, StockKey{Ingredient: m.Key.Ingredient, Location: m.To})
		}
		for _, key := range keys {
			item := items[key]
			item.Held, _ = m.after(key, item.Held)
			items[key] = item
		}
		if m.Cause == Counted {
			item := items[m.Key]
			item.Counted = m.At
			items[m.Key] = item
		}
	}
	return items
}

// HeldAt is what each Storage Location holds of the Ingredient id at a moment. A Storage
// Location no Movement of it has reached has no entry.
func (l Ledger) HeldAt(at time.Time, id int64) map[StorageLocation]Quantity {
	held := make(map[StorageLocation]Quantity, len(StorageLocations))
	for key, item := range l.At(at) {
		if key.Ingredient == id {
			held[key.Location] = item.Held
		}
	}
	return held
}

// Movements is every Movement that touched the Stock Item key, in the order they
// happened, with what it held before and after each. A Count, and a Staple's Count to
// nothing, shows what it believed before it.
func (l Ledger) Movements(key StockKey) []StockMovement {
	var (
		touched []StockMovement
		held    Quantity
	)
	for _, m := range l.movements {
		after, touches := m.after(key, held)
		if !touches {
			continue
		}
		before := held
		if m.Cause.sets() {
			before = m.Believed
		}
		touched = append(touched, StockMovement{Movement: m, Before: before, After: after})
		held = after
	}
	return touched
}

// WriteAt is when a Movement written at now, with the kitchen held, happens: now, or the
// last Movement's time if that is later, as when one that overlapped it took hold of
// the kitchen first with a clock ahead. So it is checked against all the Ledger holds,
// and comes after it.
func (l Ledger) WriteAt(now time.Time) time.Time {
	if n := len(l.movements); n > 0 && l.movements[n-1].At.After(now) {
		return l.movements[n-1].At
	}
	return now
}

// Count is the Count, written at now, that the Stock Item key holds stated, with the
// User's note: a Movement of the difference from what the Ledger believes, which keeps
// both. One that changes nothing is a Count like any other.
func (l Ledger) Count(now time.Time, key StockKey, stated Quantity, note string) Movement {
	at := l.WriteAt(now)
	believed := l.At(at)[key].Held
	return Movement{At: at, Key: key, Change: stated - believed, Cause: Counted, Believed: believed, Stated: stated, Note: note}
}

// Move is the move, written at now, of moved from the Stock Item key to the Storage
// Location to, with the User's note. When CheckMove refuses it against what key holds
// then, problem says why and there is no move.
func (l Ledger) Move(now time.Time, key StockKey, to StorageLocation, moved Quantity, note string) (_ Movement, problem Problem) {
	at := l.WriteAt(now)
	if problem = CheckMove(l.At(at)[key].Held, moved); problem != "" {
		return Movement{}, problem
	}
	return Movement{At: at, Key: key, To: to, Change: moved, Cause: Moved, Note: note}, ""
}

// CountToNothing is the Count to nothing, written at now, that marking the Ingredient id
// a Staple makes of each Stock Item holding some of it, in Storage Location order: each
// keeps what it believed and states nothing.
func (l Ledger) CountToNothing(now time.Time, id int64) []Movement {
	at := l.WriteAt(now)
	items := l.At(at)
	var counted []Movement
	for _, location := range StorageLocations {
		key := StockKey{Ingredient: id, Location: location}
		if held := items[key].Held; held > 0 {
			counted = append(counted, Movement{At: at, Key: key, Change: -held, Cause: MarkedStaple, Believed: held})
		}
	}
	return counted
}
