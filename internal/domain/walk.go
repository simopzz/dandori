package domain

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// Arrival is a Delivery taken as arrived At the end of its window, putting Put away.
type Arrival struct {
	At  time.Time
	Put []PutAwayLine
}

// PreparedPortion is a Portion a Prep Session prepares from its Planned Portion Planned,
// with its name and values per Portion.
type PreparedPortion struct {
	Planned   int64
	Name      string
	Nutrition Nutrition
}

// step is what happens at a moment, in the order steps at one moment happen.
type step int

const (
	arrives step = iota
	usesUp
	prepares
	eats
)

// happening is a step at a moment: a Delivery's arrival, a Prep Session's, or a Slot's.
type happening struct {
	at      time.Time
	step    step
	arrival Arrival
	session PrepSession
	slot    Slot
}

// UpToNow is what bringing the kitchen `l` up to `to` from just after `from` records of
// the Plan `p` by `times` and of `arrivals`: the Movements, in the order they happen, and
// the Portions its Prep Sessions prepare, listed in those Movements by PlannedID until
// they are stored. `madeFrom` gives the Portion each Planned Portion was prepared as.
// Slots and arrivals go in time order, at one moment arrival, used up, prepared, eaten.
// Each Movement is dated at its time, or the latest Movement's if that is
// later (WriteAt).
func (l Ledger) UpToNow(p Plan, madeFrom map[int64]int64, arrivals []Arrival, times SlotTimes, from, to time.Time) (written []Movement, made []PreparedPortion) {
	in := func(s Slot) bool { t := s.At(times); return t.After(from) && !t.After(to) }
	var happenings []happening
	for _, a := range arrivals {
		happenings = append(happenings, happening{at: a.At, step: arrives, arrival: a})
	}
	filled := map[string]bool{}
	fill := func(s Slot) {
		if in(s) && !filled[s.key()] {
			filled[s.key()] = true
			happenings = append(happenings, happening{at: s.At(times), step: eats, slot: s})
		}
	}
	for _, ps := range p.PrepSessions {
		if in(ps.At) {
			at := ps.At.At(times)
			happenings = append(happenings, happening{at: at, step: usesUp, session: ps}, happening{at: at, step: prepares, session: ps})
		}
		for _, pp := range ps.Portions {
			if pp.EatAt != nil {
				fill(*pp.EatAt)
			}
		}
	}
	for _, k := range p.Kept {
		fill(k.At)
	}
	slices.SortStableFunc(happenings, func(a, b happening) int { return cmp.Or(a.at.Compare(b.at), cmp.Compare(a.step, b.step)) })
	prepared := map[int64]int64{}
	maps.Copy(prepared, madeFrom)
	for _, e := range happenings {
		var ms []Movement
		switch e.step {
		case arrives:
			ms = l.Deliver(e.at, e.arrival.Put)
		case usesUp:
			ms = l.usesUp(e.at, e.session)
		case prepares:
			var portions []PreparedPortion
			ms, portions = l.prepares(e.at, e.session)
			for _, pp := range portions {
				prepared[pp.Planned] = PlannedID(pp.Planned)
			}
			made = append(made, portions...)
		case eats:
			ms = l.eats(e.at, p, e.slot, prepared)
		}
		l = l.with(ms)
		written = append(written, ms...)
	}
	return written, made
}

// with is `l` once `ms`, written after it, are.
func (l Ledger) with(ms []Movement) Ledger {
	if len(ms) == 0 {
		return l
	}
	return Ledger{movements: append(slices.Clip(l.movements), ms...), latest: ms[len(ms)-1].At}
}

// usesUp is the Movements, written at `now`, of `ps` using up each of its Ingredients but
// a Staple, as far as the kitchen holds it.
func (l Ledger) usesUp(now time.Time, ps PrepSession) []Movement {
	at := l.WriteAt(now)
	stock := map[StockKey]Quantity{}
	for key, item := range l.At(at) {
		stock[key] = item.Held
	}
	var used []Movement
	for _, ri := range ps.Ingredients() {
		if ri.Ingredient.Staple {
			continue
		}
		for _, t := range usesFrom(stock, ri) {
			used = append(used, Movement{At: at, Key: t.Key, Change: -t.Quantity, Cause: UsedUp, PrepSession: ps.ID, Slot: ps.At})
		}
	}
	return used
}

// prepares is the Movements, written at `now`, of `ps` preparing its Portions, one per
// Storage Location, and those Portions.
func (l Ledger) prepares(now time.Time, ps PrepSession) (ms []Movement, made []PreparedPortion) {
	at := l.WriteAt(now)
	name, n := ps.Name(), ps.NutritionPerPortion().Nutrition
	for _, location := range StorageLocations {
		var ids []int64
		for _, pp := range ps.Portions {
			if pp.Location == location {
				ids = append(ids, PlannedID(pp.ID))
				made = append(made, PreparedPortion{Planned: pp.ID, Name: name, Nutrition: n})
			}
		}
		if len(ids) > 0 {
			ms = append(ms, Movement{At: at, Key: StockKey{Location: location}, Portions: ids, Change: portionCount(len(ids)), Cause: Prepared, PrepSession: ps.ID, Slot: ps.At})
		}
	}
	return ms, made
}

// eats is the Movement, written at `now`, of the Portion filling the Slot `s` of `p`
// eaten from wherever the kitchen holds it, `prepared` giving each Planned Portion's
// Portion. A Portion the kitchen doesn't hold writes nothing.
func (l Ledger) eats(now time.Time, p Plan, s Slot, prepared map[int64]int64) []Movement {
	at := l.WriteAt(now)
	id, ok := p.Filling(s, prepared)
	location, held := l.PortionsAt(at)[id]
	if !ok || !held {
		return nil
	}
	return []Movement{{At: at, Key: StockKey{Location: location}, Portions: []int64{id}, Change: -portionCount(1), Cause: PortionEaten, Slot: s}}
}

// Filling is the Portion filling the Slot `s` of `p`, `madeFrom` giving the Portion each
// Planned Portion was prepared as: a Planned Portion eaten there, or a Portion without a
// Slot put there, one the kitchen held or a Planned Portion by its PlannedID, of
// whichever Cycle's Prep Session. `ok` is false when none does, or a Planned Portion
// filling it isn't prepared yet.
func (p Plan) Filling(s Slot, madeFrom map[int64]int64) (portion int64, ok bool) {
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt != nil && pp.EatAt.Equal(s) {
				portion, ok = madeFrom[pp.ID]
				return portion, ok
			}
		}
	}
	for _, k := range p.Kept {
		switch {
		case !k.At.Equal(s):
		case k.Portion > 0:
			return k.Portion, true
		default:
			portion, ok = madeFrom[PlannedID(k.Portion)]
			return portion, ok
		}
	}
	return 0, false
}
