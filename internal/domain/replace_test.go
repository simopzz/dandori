package domain_test

import (
	"slices"
	"testing"

	"github.com/simopzz/dandori/internal/domain"
)

func TestAPlanProposalReplacesThePlanFromASlotOnAndKeepsWhatWasPrepared(t *testing.T) {
	// The Cycle runs from breakfast on Mon 2 Nov to Sun 8 Nov; today is
	// Wed 4 Nov, so its Prep Sessions up to today's are taken as prepared.
	c := domain.Cycle{Start: *at("2026-11-02", domain.Breakfast), EndsOn: day("2026-11-08")}
	today := day("2026-11-04")
	tomorrow := *at("2026-11-05", domain.Breakfast)
	portion := func(id int64, eat *domain.Slot) domain.PlannedPortion {
		return domain.PlannedPortion{ID: id, Location: domain.Fridge, EatAt: eat}
	}
	session := func(id int64, at *domain.Slot, portions ...domain.PlannedPortion) domain.PrepSession {
		ps := domain.PrepSession{ID: id, Portions: portions}
		if at != nil {
			ps.At = *at
		}
		return ps
	}
	tests := []struct {
		name string
		from domain.Slot
		// own are the Cycle's Prep Sessions; others those of other Cycles
		// and from before planning.
		own, others []domain.PrepSession
		dropped     []int64
		freed       []int64
		kept        []int64
	}{
		{
			name: "today's Prep Session stays, and its Portions from today's Slot on are put in Slots again",
			from: *at("2026-11-04", domain.Dinner),
			own: []domain.PrepSession{session(1, at("2026-11-04", domain.Lunch),
				portion(11, at("2026-11-04", domain.Lunch)),
				portion(12, at("2026-11-04", domain.Dinner)),
				portion(13, at("2026-11-06", domain.Lunch)))},
			freed: []int64{12, 13},
			kept:  []int64{12, 13},
		},
		{
			name: "a Prep Session from tomorrow on is replaced, with its Portions",
			from: tomorrow,
			own: []domain.PrepSession{session(2, at("2026-11-05", domain.Dinner),
				portion(21, at("2026-11-05", domain.Dinner)),
				portion(22, at("2026-11-07", domain.Lunch)))},
			dropped: []int64{2},
		},
		{
			name: "a Portion in a Slot before from stays in it",
			from: tomorrow,
			own: []domain.PrepSession{session(3, at("2026-11-02", domain.Dinner),
				portion(31, at("2026-11-02", domain.Dinner)),
				portion(32, at("2026-11-04", domain.Lunch)),
				portion(33, at("2026-11-06", domain.Lunch)))},
			freed: []int64{33},
			kept:  []int64{33},
		},
		{
			name: "from a Slot already past, what was prepared up to today still stays",
			from: *at("2026-11-03", domain.Dinner),
			own: []domain.PrepSession{
				session(4, at("2026-11-03", domain.Dinner), portion(41, at("2026-11-03", domain.Dinner))),
				session(5, at("2026-11-05", domain.Lunch), portion(51, at("2026-11-05", domain.Lunch))),
			},
			dropped: []int64{5},
			freed:   []int64{41},
			kept:    []int64{41},
		},
		{
			name:   "a Portion carried over without a Slot is the Plan Proposal's to put in one",
			from:   tomorrow,
			others: []domain.PrepSession{session(6, at("2026-10-30", domain.Dinner), portion(61, nil))},
			kept:   []int64{61},
		},
		{
			name: "a Portion carried over in a Slot before from stays; one from from on is put in Slots again",
			from: tomorrow,
			others: []domain.PrepSession{session(7, at("2026-10-30", domain.Dinner),
				portion(71, at("2026-11-03", domain.Lunch)),
				portion(72, at("2026-11-06", domain.Dinner)))},
			freed: []int64{72},
			kept:  []int64{72},
		},
		{
			name: "a Portion of the Cycle without a Slot, eaten after it, is the Plan Proposal's to put in one",
			from: tomorrow,
			own:  []domain.PrepSession{session(8, at("2026-11-03", domain.Dinner), portion(81, nil))},
			kept: []int64{81},
		},
		{
			name: "a Portion in the next Cycle's Slot stays there",
			from: tomorrow,
			own:  []domain.PrepSession{session(9, at("2026-11-03", domain.Dinner), portion(91, at("2026-11-10", domain.Lunch)))},
		},
		{
			name:   "the next Cycle's Prep Sessions are not this Plan's to replace or keep",
			from:   tomorrow,
			others: []domain.PrepSession{session(10, at("2026-11-09", domain.Dinner), portion(101, nil))},
		},
		{
			name:   "a Prep Session from before planning has no day and stays",
			from:   tomorrow,
			others: []domain.PrepSession{session(11, nil, portion(111, nil))},
			kept:   []int64{111},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := domain.Replace(c, tt.from, today, tt.own, tt.others)
			if r.From != tt.from || r.Prepared != tomorrow {
				t.Errorf("Replace from %s, prepared from %s; want %s and %s", r.From, r.Prepared, tt.from, tomorrow)
			}
			var kept []int64
			for _, k := range r.Kept {
				kept = append(kept, k.ID)
			}
			for _, got := range []struct {
				what      string
				got, want []int64
			}{
				{"dropped", r.Dropped, tt.dropped},
				{"freed", r.Freed, tt.freed},
				{"kept", kept, tt.kept},
			} {
				if !slices.Equal(got.got, got.want) {
					t.Errorf("%s = %v, want %v", got.what, got.got, got.want)
				}
			}
		})
	}
}

func TestPortionsKeptAreInOrderWithWhatTheyAre(t *testing.T) {
	c := domain.Cycle{Start: *at("2026-11-02", domain.Breakfast), EndsOn: day("2026-11-08")}
	stew := &domain.Recipe{Name: "Stew", Portions: 2}
	own := []domain.PrepSession{{ID: 1, At: *at("2026-11-02", domain.Dinner), Recipe: stew, Portions: []domain.PlannedPortion{{ID: 14}, {ID: 12}}}}
	others := []domain.PrepSession{{ID: 2, At: *at("2026-10-30", domain.Dinner), Recipe: stew, Portions: []domain.PlannedPortion{{ID: 13}}}}

	got := domain.Replace(c, *at("2026-11-05", domain.Breakfast), day("2026-11-04"), own, others).Kept
	want := []domain.KeptPortion{
		{ID: 12, Name: "Stew"},
		{ID: 13, Name: "Stew"},
		{ID: 14, Name: "Stew"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Kept = %+v, want %+v", got, want)
	}
}
