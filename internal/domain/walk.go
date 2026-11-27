package domain

import (
	"cmp"
	"maps"
	"slices"
	"time"
)

// Arrival is a Delivery taken as arrived At the end of its window, putting Put away, and
// whether it is Ordered: an evening's thaw counts one still to come as held.
type Arrival struct {
	At      time.Time
	Put     []PutAwayLine
	Ordered bool
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
	thaws
)

// happening is a step at a moment: a Delivery's arrival, a Prep Session's, a Slot's, or
// an evening's thaw for the day `day`.
type happening struct {
	day     time.Time
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
// Slots and arrivals go in time order, at one moment arrival, used up, prepared, eaten,
// and at dinner's time of day the thaw for the next day of `p` (thaw). An arrival after
// `to` doesn't happen: an evening's thaw counts it held. Each Movement is dated at its
// time, or the latest Movement's if that is later (WriteAt).
func (l Ledger) UpToNow(p Plan, madeFrom map[int64]int64, arrivals []Arrival, times Times, from, to time.Time) (written []Movement, made []PreparedPortion) {
	in := func(s Slot) bool { t := s.At(times); return t.After(from) && !t.After(to) }
	var happenings []happening
	for _, a := range arrivals {
		if !a.At.After(to) {
			happenings = append(happenings, happening{at: a.At, step: arrives, arrival: a})
		}
	}
	filled, evenings := map[string]bool{}, map[string]bool{}
	thawFor := func(day time.Time) {
		evening := ThawEvening(day)
		if in(evening) && !evenings[evening.key()] {
			evenings[evening.key()] = true
			happenings = append(happenings, happening{at: evening.At(times), step: thaws, day: day})
		}
	}
	fill := func(s Slot) {
		thawFor(s.On)
		if in(s) && !filled[s.key()] {
			filled[s.key()] = true
			happenings = append(happenings, happening{at: s.At(times), step: eats, slot: s})
		}
	}
	for _, ps := range p.PrepSessions {
		thawFor(ps.At.On)
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
		case thaws:
			ms = l.thaw(e.at, p, e.day, arrivals, prepared, times)
		}
		l = l.with(ms)
		written = append(written, ms...)
	}
	return written, made
}

// thaw is the Movements, written at `now`, of the evening's thaw into the fridge for the
// day `day` of `p` by `times`, `prepared` giving each Planned Portion's Portion: what the
// day's Prep Sessions thaw (Plan.Thaws), one Movement each per Ingredient, with each of
// `arrivals` still to come, Ordered and due before the day's first Slot, held where it is
// put away, and every frozen Portion filling one of its Slots. A freezer split of an
// Ingredient not used from frozen isn't held: there is no evening left to thaw it.
func (l Ledger) thaw(now time.Time, p Plan, day time.Time, arrivals []Arrival, prepared map[int64]int64, times Times) []Movement {
	at := l.WriteAt(now)
	var sessions []PrepSession
	var slots []Slot
	for _, ps := range p.PrepSessions {
		if ps.At.On.Equal(day) {
			sessions = append(sessions, ps)
			slots = append(slots, ps.At)
		}
		for _, pp := range ps.Portions {
			if pp.EatAt != nil && pp.EatAt.On.Equal(day) {
				slots = append(slots, *pp.EatAt)
			}
		}
	}
	for _, k := range p.Kept {
		if k.At.On.Equal(day) {
			slots = append(slots, k.At)
		}
	}
	slices.SortStableFunc(slots, func(a, b Slot) int { return a.At(times).Compare(b.At(times)) })
	slots = slices.CompactFunc(slots, Slot.Equal)
	stock := l.stock(at)
	for _, a := range arrivals {
		if !a.Ordered || !a.At.After(at) || len(slots) == 0 || !a.At.Before(slots[0].At(times)) {
			continue
		}
		for _, put := range a.Put {
			if put.Location != Freezer || put.Ingredient.UseFromFrozen {
				stock[StockKey{Ingredient: put.Ingredient.ID, Location: put.Location}] += put.Quantity
			}
		}
	}
	var ms []Movement
	for _, t := range (Plan{PrepSessions: sessions}).Thaws(stock) {
		ms = append(ms, Movement{At: at, Key: StockKey{Ingredient: t.ingredient.ID, Location: Freezer}, To: Fridge, Change: t.quantity, Cause: Thawed, PrepSession: t.prepSession, Slot: t.slot})
	}
	where := l.PortionsAt(at)
	for _, s := range slots {
		if id, ok := p.Filling(s, prepared); ok && where[id] == Freezer {
			ms = append(ms, Movement{At: at, Key: StockKey{Location: Freezer}, To: Fridge, Portions: []int64{id}, Change: portionCount(1), Cause: Thawed, Slot: s})
		}
	}
	return ms
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
	stock := l.stock(at)
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

// TakesFrom is where each of `sessions`, in turn, would take each of its Ingredients
// from, using them up at `at`, by Ingredient id: the Storage Locations of the Stock Items
// it takes from, in order, none for one the kitchen holds none of once the sessions
// before it took their share. A Staple has no entry.
func (l Ledger) TakesFrom(at time.Time, sessions []PrepSession) []map[int64][]StorageLocation {
	stock := l.stock(at)
	out := make([]map[int64][]StorageLocation, len(sessions))
	for i, ps := range sessions {
		out[i] = map[int64][]StorageLocation{}
		for _, ri := range ps.Ingredients() {
			if ri.Ingredient.Staple {
				continue
			}
			out[i][ri.Ingredient.ID] = []StorageLocation{}
			for _, t := range usesFrom(stock, ri) {
				out[i][ri.Ingredient.ID] = append(out[i][ri.Ingredient.ID], t.Key.Location)
			}
		}
	}
	return out
}

// stock is what each Stock Item of `l` holds at `at`.
func (l Ledger) stock(at time.Time) map[StockKey]Quantity {
	stock := map[StockKey]Quantity{}
	for key, item := range l.At(at) {
		stock[key] = item.Held
	}
	return stock
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

// Filled is the Slot each Portion fills in `p`, by its id, as Filling finds it with
// `madeFrom`: the earliest, for one filling more than one.
func (p Plan) Filled(madeFrom map[int64]int64) map[int64]Slot {
	filled := map[int64]Slot{}
	fill := func(s Slot) {
		if id, ok := p.Filling(s, madeFrom); ok {
			if was, seen := filled[id]; !seen || s.Before(was) {
				filled[id] = s
			}
		}
	}
	for _, ps := range p.PrepSessions {
		for _, pp := range ps.Portions {
			if pp.EatAt != nil {
				fill(*pp.EatAt)
			}
		}
	}
	for _, k := range p.Kept {
		fill(k.At)
	}
	return filled
}
